//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|             trading algorithm x3.100-2026

// levi implements an intraday momentum breakout strategy
// - Monitor numerous stocks for breakout patterns
// - Enter long when price breaks above N-minute high
// - Hold multiple positions concurrently until margin exhausted
// - Use per-position trailing stops for exits
// - Close all positions before market close

package main

import (
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"dropbear/symbol"
	"flag"
	"log"
)

var (
	flagSymbols   = flag.String("symbol", "GOOG", "symbols to monitor")
	flagRound     = flag.Bool("round", true, "use round lot order quantities")
	flagSlipOpen  = decimal.FlagBPS("slip-open", "25", "max slippage on opening trade")
	flagSlipClose = decimal.FlagBPS("slip-close", "25", "max slippage on closing trade")
	flagMinPrice  = decimal.Flag("minprice", "10", "minimum share price to trade")
	flagMaxDD     = decimal.FlagPercent("maxdd", "9", "max daily drawdown before halting")
	flagMaxPos    = decimal.FlagPercent("maxpos", "50", "max position size as percent of portfolio")
	flagMaxLiq    = decimal.FlagPercent("maxliq", "5", "max position as percent of today's dollar volume")
)

const (
	stopOpeningTime  = 30 * clocky.Minute
	startClosingTime = 15 * clocky.Minute
)

var (
	gTraders  map[symbol.Symbol]*Trader
	gDayStart decimal.Decimal // portfolio value at start of day
	gHalted   bool            // true if we hit max drawdown today
	gDate     int
)

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	syms, err := symbol.Expand(*flagSymbols)
	if err != nil {
		log.Fatalf("invalid symbols: %v", err)
	}
	if len(syms) == 0 {
		log.Fatal("no symbols specified")
	}

	gTraders = make(map[symbol.Symbol]*Trader)

	for _, sym := range syms {
		if gTraders[sym] != nil {
			continue // dedupe
		}
		symStr := sym.String()
		_, ok := OptimalParams[symStr]
		if !ok {
			log.Printf("paramgen tool wasn't run for symbol %s", sym)
			continue
		}
		equity, err := cubby.AddEquity(sym)
		if err != nil {
			log.Printf("error adding symbol %s: %v", sym, err)
			continue
		}
		trader := &Trader{
			equity:      equity,
			maxHigh:     indicators.NewMax(clocky.Duration(OptimalParams[symStr].Lookback)*clocky.Minute - 1),
			dayVolumeMA: indicators.NewWWMA(12),
		}
		gTraders[sym] = trader
		equity.OnBar = trader.onBar
	}

	log.Printf("Multi-Position Day Trading Strategy")
	log.Printf("  Symbols: %d", len(gTraders))
	log.Printf("  Minimum Share Price: $%s", flagMinPrice)
	log.Printf("  Max Slippage Open: %s bps", flagSlipOpen.BPS())
	log.Printf("  Max Slippage Close: %s bps", flagSlipClose.BPS())
	log.Printf("  Max Daily Drawdown: %s%%", flagMaxDD.MulInt(100))
	log.Printf("  Max Position Size: %s%% of portfolio", flagMaxPos.MulInt(100))
	log.Printf("  Max Participation: %s%% of daily volume", flagMaxLiq.MulInt(100))

	cubby.Run()
}

type Trader struct {
	order          *cubby.Order
	equity         *cubby.Equity
	maxHigh        *indicators.Max // rolling max of bar highs
	highSince      decimal.Decimal // highest price since entry (for trailing stop)
	dayVolume      decimal.Decimal
	myDayVolume    decimal.Decimal
	dayVolumeMA    *indicators.WWMA
	dayVolumeSince clocky.Time
	monotonicTime  clocky.Time
}

func (state *Trader) onBar(c *ds.Bar) {

	// check any current order status
	if state.order != nil && state.order.Status.IsFinal() {
		state.myDayVolume = state.myDayVolume.Add(state.order.FilledQuantity)
		state.order = nil
	}

	// only pay attention to monotonic time bars
	if !c.Timestamp.After(state.monotonicTime) {
		return
	}
	state.monotonicTime = c.Timestamp

	// check day
	date := c.Timestamp.DateInt()
	if date != gDate {
		onDayChange()
		gDate = date
	}

	// update indicators
	lastHigh := state.maxHigh.Value
	state.maxHigh.Add(c.Timestamp, c.High)
	state.dayVolume = state.dayVolume.Add(c.Volume)
	if state.dayVolumeSince.IsZero() {
		state.dayVolumeSince = c.Timestamp
	}
	if !state.maxHigh.IsReady() {
		return
	}
	if cubby.IsWarmingUp {
		return
	}

	// check time
	now := clocky.Now()
	openTime := cboe.GetOpenTime(now)
	closeTime := cboe.GetCloseTime(now)
	if now < openTime || now >= closeTime {
		return
	}
	if closeTime.Sub(now) <= startClosingTime {
		state.closePosition()
		return
	}

	// capture starting portfolio value for drawdown tracking
	if gDayStart.IsZero() {
		gDayStart = cubby.GetPortfolioValue()
		if cubby.Verbose {
			log.Printf("\033[1mopening bell portfolio value $%s\033[0m", gDayStart.FormatThousand(2))
		}
	}

	// if halted, only process EOD closes (handled above)
	if gHalted {
		state.closePosition()
		return
	}

	// check for max daily drawdown
	portfolioValue := cubby.GetPortfolioValue()
	drawdown := gDayStart.Sub(portfolioValue).Div(gDayStart)
	if drawdown.Cmp(*flagMaxDD) >= 0 {
		if cubby.Verbose {
			log.Printf("halting all trading today because drawdown %.2f%% exceeds max %.2f%%",
				drawdown.MulInt(100).Float64(), flagMaxDD.MulInt(100).Float64())
		}
		// liquidate and suspend trading
		for _, trader := range gTraders {
			trader.closePosition()
		}
		gHalted = true
		return
	}

	// check trailing stop
	price := c.Close
	if !state.equity.Quantity.IsZero() {
		state.highSince = state.highSince.Max(price)
		params := OptimalParams[state.equity.Symbol.String()]
		stopPrice := state.highSince.Mul(decimal.One.Sub(params.Trail))
		if price.Cmp(stopPrice) <= 0 {
			state.closePosition()
		}
		return // already holding, don't add more
	}

	// check if we already have an open order to open with no fills
	if state.order != nil {
		// cancel if we've waited too long
		if c.Timestamp.Sub(state.order.OrderedAt) >= *cubby.FlagPatience {
			err := state.order.Cancel()
			if err != nil {
				log.Printf("error cancelling existing order for %s: %v", state.equity.Symbol, err)
			}
			state.myDayVolume = state.myDayVolume.Add(state.order.FilledQuantity)
			state.order = nil
		}
		return
	}

	// don't open new positions on penny stocks
	if price.Cmp(*flagMinPrice) < 0 {
		return
	}

	// don't open new positions near end of the day
	if closeTime.Sub(now) <= stopOpeningTime {
		return
	}

	// check for breakout entry
	state.checkBreakoutEntry(price, lastHigh)
}

func (state *Trader) checkBreakoutEntry(price, high decimal.Decimal) {

	// check for breakout: price above lookback high by minimum gap
	params := OptimalParams[state.equity.Symbol.String()]
	breakoutThreshold := high.Mul(decimal.One.Add(params.MinGap))
	if price.Cmp(breakoutThreshold) <= 0 {
		return
	}

	// calculate limit price with slippage allowance
	limitPrice := price.Mul(decimal.One.Add(*flagSlipOpen))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	// calculate position size based on available margin
	maxQty := state.equity.GetMaxOrderQuantity(limitPrice)
	if !maxQty.IsPositive() {
		if cubby.Verbose {
			log.Printf("not entering %s because no margin is available", state.equity.Symbol)
		}
		return // no margin available
	}

	// limit position size to maxpos percent of portfolio
	maxValue := cubby.GetPortfolioValue().Mul(*flagMaxPos)
	maxQtyByPos := maxValue.Div(limitPrice)
	maxQty = maxQty.Min(maxQtyByPos)
	if !maxQty.IsPositive() {
		if cubby.Verbose {
			log.Printf("not entering %s because position size limit is $%s shares",
				state.equity.Symbol, maxValue.FormatThousand(2))
		}
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

	// don't participate in too much of stock's daily volume
	avgDayVolume := state.getAverageDailyVolume()
	maxQtyByLiq := avgDayVolume.Mul(*flagMaxLiq).Truncate()
	maxQtyByLiqRemaining := maxQtyByLiq.Sub(state.myDayVolume)
	maxQty = maxQty.Min(maxQtyByLiqRemaining)
	if !maxQty.IsPositive() {
		if cubby.Verbose {
			log.Printf("not entering %s because daily volume is %s shares and my participation limit is %s shares",
				state.equity.Symbol, avgDayVolume, maxQtyByLiq)
		}
		return
	}

	// round order quantity to lot size
	maxQty = RoundToLotSize(maxQty, limitPrice)
	if !maxQty.IsPositive() {
		return
	}

	// give the order to open position
	order, err := state.equity.Order(maxQty, limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("error placing opening order for %s: %v", state.equity.Symbol, err)
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

	// cancel any pending order to open position
	if t.order != nil && t.order.Status.IsOpen() {
		if t.order.Side == ds.SideSell && shares.IsPositive() {
			return // already selling to close
		}
		if t.order.Side == ds.SideBuy && shares.IsNegative() {
			return // already buying to close
		}
		err := t.order.Cancel()
		if err != nil {
			log.Printf("error cancelling existing order for %s: %v", t.equity.Symbol, err)
		}
		t.myDayVolume = t.myDayVolume.Add(t.order.FilledQuantity)
		t.order = nil
	}

	// close position with slippage allowance
	limitPrice := t.equity.Price
	if shares.IsPositive() {
		limitPrice = limitPrice.Mul(decimal.One.Sub(*flagSlipClose))
	} else {
		limitPrice = limitPrice.Mul(decimal.One.Add(*flagSlipClose))
	}
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)
	order, err := t.equity.Order(shares.Neg(), limitPrice)
	if err != nil {
		log.Printf("error closing position %s: %v", t.equity.Symbol, err)
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
		if state.dayVolumeSince.ClockInt() <= 7_00_00 {
			state.dayVolumeMA.Add(state.dayVolume)
		}
		state.dayVolumeSince = 0
		state.myDayVolume = decimal.Zero
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
