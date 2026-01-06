//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|            trading algorithm x3.100-2026

// fngu implements a multi-position day trading strategy for leveraged etfs
//
// Strategy: Intraday momentum breakout scanner
//   - Monitor multiple symbols for breakout patterns
//   - Enter long when price breaks above N-minute high
//   - Hold multiple positions concurrently until margin exhausted
//   - Use per-position trailing stops for exits
//   - Close all positions before market close
//
// Usage:
//
//	go run ./cmd/fngu -backtest -v -start 2025-10-01 -symbol etc/picks/leverage -cash 161844

package main

import (
	"dropbear/clocky"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"flag"
	"log"
	"strings"
)

var (
	flagSymbols   = flag.String("symbol", "FNGU", "symbols to monitor")
	flagBenchmark = flag.String("benchmark", "QQQ", "benchmark symbol")
	flagRound     = flag.Bool("round", true, "use round lot order quantities")
	flagSlip      = decimal.FlagBPS("slip", "100", "max slippage willing to pay in basis points")
	flagMinPrice  = decimal.Flag("minprice", "10", "minimum share price to trade")
	flagMaxDD     = decimal.FlagPercent("maxdd", "9", "max daily drawdown before halting")
	flagMaxPos    = decimal.FlagPercent("maxpos", "50", "max position size as percent of portfolio")
	flagMaxLiq    = decimal.FlagPercent("maxliq", "5", "max position as percent of today's dollar volume")
)

const (
	openTime  = 6_45_00
	closeTime = 12_45_00
)

var (
	gTraders     map[string]*Trader
	gCurrentDate int
	gIsOpen      bool
	gDayStart    decimal.Decimal // portfolio value at start of day
	gHalted      bool            // true if we hit max drawdown today
)

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	symbols := strings.Fields(*flagSymbols)
	if len(symbols) == 0 {
		log.Fatal("no symbols specified")
	}

	gTraders = make(map[string]*Trader)

	for _, sym := range symbols {
		if gTraders[sym] != nil {
			continue // dedupe
		}
		_, ok := OptimalParams[sym]
		if !ok {
			log.Printf("paramgen tool wasn't run for symbol %s", sym)
			continue
		}
		equity, err := cubby.AddEquity(sym)
		if err != nil {
			log.Printf("error adding symbol %s: %v", sym, err)
			continue
		}
		state := &Trader{
			equity:      equity,
			maxHigh:     indicators.NewMax(clocky.Duration(OptimalParams[sym].Lookback)*clocky.Minute - 1),
			dayVolumeMA: indicators.NewWWMA(12),
		}
		gTraders[sym] = state
		equity.OnBar = state.onBar
	}

	benchmark, _ := cubby.AddEquity(*flagBenchmark)
	cubby.Benchmark = benchmark

	log.Printf("Multi-Position Day Trading Strategy")
	log.Printf("  Symbols: %d", len(gTraders))
	log.Printf("  Slippage: %s bps", flagSlip.MulInt(10000).Format(0))

	cubby.Run()
}

type Trader struct {
	equity       *cubby.Equity
	maxHigh      *indicators.Max // rolling max of bar highs
	highSince    decimal.Decimal // highest price since entry (for trailing stop)
	lookbackHigh decimal.Decimal // max high before current bar (for breakout detection)
	dayHigh      decimal.Decimal
	dayLow       decimal.Decimal
	dayVolume    decimal.Decimal
	myDayVolume  decimal.Decimal
	dayVolumeMA  *indicators.WWMA
	order        *cubby.Order
}

func (state *Trader) onBar(c *ds.Bar) {

	// check order status
	if state.order != nil && state.order.Status.IsFinal() {
		state.myDayVolume = state.myDayVolume.Add(state.order.FilledQuantity)
		state.order = nil
	}

	// check date change
	date := c.Timestamp.DateInt()
	if date != gCurrentDate {
		onDayChange()
		gCurrentDate = date
	}

	// update rolling max indicator (save previous value for breakout detection)
	state.lookbackHigh = state.maxHigh.Value
	state.maxHigh.Add(c.Timestamp, c.High)

	// accumulate volume
	state.dayVolume = state.dayVolume.Add(c.Volume)

	// update day high/low
	if state.dayHigh.IsZero() || c.High.Cmp(state.dayHigh) > 0 {
		state.dayHigh = c.High
	}
	if state.dayLow.IsZero() || c.Low.Cmp(state.dayLow) < 0 {
		state.dayLow = c.Low
	}

	if !state.maxHigh.IsReady() {
		return
	}

	time := c.Timestamp.ClockInt()
	if time < openTime {
		return
	}
	if time >= closeTime {
		// Keep trying to close until flat
		if !state.equity.Quantity.IsZero() {
			state.closePosition()
		}
		return
	}

	gIsOpen = true
	price := c.Close

	// capture starting portfolio value for drawdown tracking
	if gDayStart.IsZero() {
		gDayStart = cubby.GetPortfolioValue()
		if *cubby.FlagVerbose {
			log.Printf("\033[1mOPEN: portfolio value $%s\033[0m", gDayStart.FormatThousand(2))
		}
	}

	// check for max daily drawdown
	if !gHalted && !gDayStart.IsZero() {
		current := cubby.GetPortfolioValue()
		drawdown := gDayStart.Sub(current).Div(gDayStart)
		if drawdown.Cmp(*flagMaxDD) >= 0 {
			gHalted = true
			if *cubby.FlagVerbose {
				log.Printf("\033[1;31mHALTED: drawdown %.2f%% exceeds max %.2f%%\033[0m",
					drawdown.MulInt(100).Float64(), flagMaxDD.MulInt(100).Float64())
			}
			// Liquidate all positions
			for _, trader := range gTraders {
				trader.closePosition()
			}
			return
		}
	}

	// if halted, only process EOD closes (handled above)
	if gHalted {
		return
	}

	// check trailing stop if we hold this symbol
	if !state.equity.Quantity.IsZero() {
		state.highSince = state.highSince.Max(price)
		params := OptimalParams[state.equity.Symbol]
		stopPrice := state.highSince.Mul(decimal.One.Sub(params.Trail))
		if price.Cmp(stopPrice) <= 0 {
			state.closePosition()
		}
		return // already holding, don't add more
	}

	// never enter penny stocks (per-share fees are devastating)
	if price.Cmp(*flagMinPrice) < 0 {
		return
	}

	// check for breakout entry
	state.checkBreakoutEntry(c)
}

func (state *Trader) checkBreakoutEntry(c *ds.Bar) {

	// use lookback high (max before current bar, computed in onBar)
	if state.lookbackHigh.IsZero() {
		return
	}

	price := c.Close

	// check for breakout: price above lookback high by minimum gap
	params := OptimalParams[state.equity.Symbol]
	breakoutThreshold := state.lookbackHigh.Mul(decimal.One.Add(params.MinGap))
	if price.Cmp(breakoutThreshold) <= 0 {
		return
	}

	// calculate limit price with slippage allowance
	limitPrice := price.Mul(decimal.One.Add(*flagSlip))

	// calculate position size based on available margin
	maxQty := state.equity.GetMaxOrderQuantity(limitPrice)
	if !maxQty.IsPositive() {
		return // no margin available
	}

	// limit position size to maxpos percent of portfolio
	maxValue := cubby.GetPortfolioValue().Mul(*flagMaxPos)
	maxQtyByPos := maxValue.Div(limitPrice)
	maxQty = maxQty.Min(maxQtyByPos)
	if !maxQty.IsPositive() {
		return
	}

	// cancel existing order if any
	if state.order != nil && state.order.Status.IsOpen() {
		err := state.order.Cancel()
		if err != nil {
			log.Printf("error cancelling existing order for %s: %v", state.equity.Symbol, err)
		}
		state.myDayVolume = state.myDayVolume.Add(state.order.FilledQuantity)
		state.order = nil
	}

	// don't participate in more than a certain percent of a stock's daily volume
	avgDayVolume := state.getAverageDailyVolume()
	maxQtyByLiq := avgDayVolume.Mul(*flagMaxLiq).Truncate()
	maxQtyByLiqRemaining := maxQtyByLiq.Sub(state.myDayVolume)
	maxQty = maxQty.Min(maxQtyByLiqRemaining)
	if !maxQty.IsPositive() {
		return
	}

	// round to lot size if needed
	maxQty = RoundToLotSize(maxQty, limitPrice)
	if !maxQty.IsPositive() {
		return
	}

	// place IOC limit order
	order, err := state.equity.Order(maxQty, limitPrice)
	if err != nil {
		if *cubby.FlagVerbose {
			log.Printf("\033[31merror placing buy order for %s: %v\033[0m", state.equity.Symbol, err)
		}
		return
	}
	state.highSince = state.equity.Price
	state.order = order
}

func (t *Trader) closePosition() {
	shares := t.equity.Quantity
	if shares.IsZero() {
		return
	}

	// cancel existing order if any
	if t.order != nil && t.order.Status.IsOpen() {
		err := t.order.Cancel()
		if err != nil {
			log.Printf("error cancelling existing order for %s: %v", t.equity.Symbol, err)
		}
		t.myDayVolume = t.myDayVolume.Add(t.order.FilledQuantity)
		t.order = nil
	}

	// sell with slippage allowance (willing to accept slip% below current price)
	limitPrice := t.equity.Price.Mul(decimal.One.Sub(*flagSlip))
	order, err := t.equity.Order(shares.Neg(), limitPrice)
	if err != nil {
		log.Printf("error closing %s: %v", t.equity.Symbol, err)
		return
	}
	t.order = order
}

func (t *Trader) getAverageDailyVolume() decimal.Decimal {
	if t.dayVolumeMA.Value.IsPositive() {
		return t.dayVolumeMA.Value
	}
	return t.dayVolume // first day
}

func onDayChange() {
	for _, state := range gTraders {
		params := OptimalParams[state.equity.Symbol]
		lookback := params.Lookback
		state.maxHigh = indicators.NewMax(clocky.Duration(lookback)*clocky.Minute - 1)
		state.lookbackHigh = decimal.Zero
		state.dayHigh = decimal.Zero
		state.dayLow = decimal.Zero
		state.myDayVolume = decimal.Zero
		state.dayVolumeMA.Add(state.dayVolume)
		state.dayVolume = decimal.Zero
	}
	gDayStart = decimal.Zero
	gHalted = false
}

func RoundToLotSize(shares, price decimal.Decimal) decimal.Decimal {
	if *flagRound {
		return shares.QuantizeTruncate(decimal.FromInt(100))
	}
	return shares.QuantizeTruncate(decimal.One)
}
