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
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/indicators"
	"dropbear/loggy"
	"flag"
	"log"
	"strings"
)

var (
	flagSymbols   = flag.String("symbol", "FNGU", "symbols to monitor")
	flagBenchmark = flag.String("benchmark", "QQQ", "benchmark symbol")
	flagCash      = decimal.Flag("cash", "100_000", "initial USD balance")
	flagSlip      = decimal.FlagBPS("slip", "50", "max slippage willing to pay in basis points")
	flagMinPrice  = decimal.Flag("minprice", "10", "minimum share price to trade")
	flagMaxDD     = decimal.FlagPercent("maxdd", "9", "max daily drawdown before halting")
	flagMaxPos    = decimal.FlagPercent("maxpos", "50", "max position size as percent of portfolio")
	flagMaxLiq    = decimal.FlagPercent("maxliq", "5", "max position as percent of today's dollar volume")
)

const (
	openTime  = 6_45_00
	closeTime = 12_45_00
)

// position tracks an open position with its trailing stop state
type position struct {
	equity    *cubby.Equity
	highSince decimal.Decimal // highest price since entry (for trailing stop)
}

// symbolState tracks per-symbol state for the scanner
type symbolState struct {
	equity       *cubby.Equity
	maxHigh      *indicators.Max // rolling max of bar highs
	lookbackHigh decimal.Decimal // max high before current bar (for breakout detection)
	dayHigh      decimal.Decimal
	dayLow       decimal.Decimal
	kiloVolume   decimal.Decimal // cumulative dollar volume today in $1000s
}

var (
	gSymbols     map[string]*symbolState
	gPositions   map[string]*position // currently held positions by symbol
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

	gSymbols = make(map[string]*symbolState)
	gPositions = make(map[string]*position)

	for _, sym := range symbols {
		if gSymbols[sym] != nil {
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
		state := &symbolState{
			equity:  equity,
			maxHigh: indicators.NewMax(clocky.Duration(OptimalParams[sym].Lookback)*clocky.Minute - 1),
		}
		gSymbols[sym] = state
		equity.OnBar = func(s *symbolState) func(*alpaca.Bar) {
			return func(c *alpaca.Bar) {
				onBar(s, c)
			}
		}(state)
	}

	benchmark, _ := cubby.AddEquity(*flagBenchmark)
	cubby.Benchmark = benchmark

	cubby.Cash = *flagCash

	log.Printf("Multi-Position Day Trading Strategy")
	log.Printf("  Symbols: %d", len(gSymbols))
	log.Printf("  Slippage: %s bps", flagSlip.MulInt(10000).Format(0))

	cubby.Run()
}

func onBar(state *symbolState, c *alpaca.Bar) {
	date := c.Timestamp.DateInt()
	if date != gCurrentDate {
		onDayChange()
		gCurrentDate = date
	}

	// Update rolling max indicator (save previous value for breakout detection)
	state.lookbackHigh = state.maxHigh.Value
	state.maxHigh.Add(c.Timestamp, c.High)

	// Accumulate dollar volume (volume * VWAP, or volume * close if no VWAP)
	vwap := c.VWAP
	if vwap.IsZero() {
		vwap = c.Close
	}
	state.kiloVolume = state.kiloVolume.Add(c.Volume.Mul(vwap).DivInt(1000))

	// Update day high/low
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
			closePosition(state.equity, "EOD")
		}
		return
	}

	gIsOpen = true
	price := c.Close

	// Capture starting portfolio value for drawdown tracking
	if gDayStart.IsZero() {
		gDayStart = cubby.GetPortfolioValue()
		if *cubby.FlagVerbose {
			log.Printf("\033[1mOPEN: portfolio value $%s\033[0m", gDayStart.FormatThousand(2))
		}
	}

	// Check for max daily drawdown
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
			for _, pos := range gPositions {
				closePosition(pos.equity, "DRAWDOWN_HALT")
			}
			return
		}
	}

	// If halted, only process EOD closes (handled above)
	if gHalted {
		return
	}

	// Check trailing stop if we hold this symbol
	if pos, ok := gPositions[state.equity.Symbol]; ok {
		if price.Cmp(pos.highSince) > 0 {
			pos.highSince = price
		}
		params := OptimalParams[state.equity.Symbol]
		stopPrice := pos.highSince.Mul(decimal.One.Sub(params.Trail))
		if price.Cmp(stopPrice) <= 0 {
			closePosition(state.equity, "TRAIL_STOP")
		}
		return // already holding, don't add more
	}

	// Never enter penny stocks (per-share fees are devastating)
	if price.Cmp(*flagMinPrice) < 0 {
		return
	}

	// Check for breakout entry
	checkBreakoutEntry(state, c)
}

func checkBreakoutEntry(state *symbolState, c *alpaca.Bar) {
	// Use lookback high (max before current bar, computed in onBar)
	if state.lookbackHigh.IsZero() {
		return
	}

	price := c.Close

	// Check for breakout: price above lookback high by minimum gap
	params := OptimalParams[state.equity.Symbol]
	breakoutThreshold := state.lookbackHigh.Mul(decimal.One.Add(params.MinGap))
	if price.Cmp(breakoutThreshold) <= 0 {
		return
	}

	// Calculate limit price with slippage allowance
	limitPrice := price.Mul(decimal.One.Add(*flagSlip))

	// Calculate position size based on available margin
	maxQty := state.equity.GetMaxOrderQuantity(limitPrice)
	if !maxQty.IsPositive() {
		return // no margin available
	}

	// Limit position size to maxpos percent of portfolio
	maxValue := cubby.GetPortfolioValue().Mul(*flagMaxPos)
	maxQtyByPos := maxValue.Div(limitPrice).QuantizeTruncate(decimal.One)
	if maxQtyByPos.Cmp(maxQty) < 0 {
		maxQty = maxQtyByPos
	}
	if !maxQty.IsPositive() {
		return
	}

	// Limit position size to maxliq percent of today's dollar volume (liquidity check)
	if state.kiloVolume.IsPositive() {
		maxKiloVol := state.kiloVolume.Mul(*flagMaxLiq)
		maxQtyByLiq := maxKiloVol.Div(limitPrice.DivInt(1000)).QuantizeTruncate(decimal.One)
		if maxQtyByLiq.Cmp(maxQty) < 0 {
			maxQty = maxQtyByLiq
			if *cubby.FlagVerbose && maxQty.IsPositive() {
				log.Printf("LIQUIDITY: %s capped to %s shares ($%sk is %s%% of $%sk vol)",
					state.equity.Symbol, maxQty,
					maxKiloVol.FormatThousand(3),
					flagMaxLiq.MulInt(100).Format(1),
					state.kiloVolume.FormatThousand(3))
			}
		}
		if !maxQty.IsPositive() {
			log.Printf("LIQUIDITY: %s no shares can be bought within liquidity limits ($%sk is %s%% of $%sk vol)",
				state.equity.Symbol,
				maxKiloVol.FormatThousand(3),
				flagMaxLiq.MulInt(100).Format(1),
				state.kiloVolume.FormatThousand(3))
			return
		}
	}

	// Place IOC limit order
	order, err := state.equity.LimitOrder(maxQty, limitPrice, alpaca.TimeInForceIOC)
	if err != nil {
		if *cubby.FlagVerbose {
			log.Printf("\033[31merror placing buy order for %s: %v\033[0m", state.equity.Symbol, err)
		}
		return
	}
	order.Log()

	if order.FilledQuantity.IsZero() {
		return
	}

	// Track position with trailing stop
	gPositions[state.equity.Symbol] = &position{
		equity:    state.equity,
		highSince: order.FilledPrice,
	}
}

func closePosition(equity *cubby.Equity, reason string) {
	shares := equity.Quantity
	if shares.IsZero() {
		delete(gPositions, equity.Symbol)
		return
	}

	// Sell with slippage allowance (willing to accept slip% below current price)
	limitPrice := equity.Price.Mul(decimal.One.Sub(*flagSlip))
	order, err := equity.LimitOrder(shares.Neg(), limitPrice, alpaca.TimeInForceIOC)
	if err != nil {
		log.Printf("error closing %s: %v", equity.Symbol, err)
		return
	}
	order.Reason = reason
	order.Log()

	// If fully closed, remove from positions
	if equity.Quantity.IsZero() {
		delete(gPositions, equity.Symbol)
	}
}

func onDayChange() {
	for _, state := range gSymbols {
		params := OptimalParams[state.equity.Symbol]
		lookback := params.Lookback
		if lookback == 0 {
			lookback = 9 // default
		}
		state.maxHigh = indicators.NewMax(clocky.Duration(lookback)*clocky.Minute - 1)
		state.lookbackHigh = decimal.Zero
		state.dayHigh = decimal.Zero
		state.dayLow = decimal.Zero
		state.kiloVolume = decimal.Zero
	}
	gPositions = make(map[string]*position)
	gDayStart = decimal.Zero
	gHalted = false
}
