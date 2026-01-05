// Command fngu implements a multi-position day trading strategy for leveraged ETFs.
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
//	go run ./cmd/fngu -backtest -symbol "NVDL SOXL GUSH"
//	go run ./cmd/fngu -backtest -symbol "NVDL SOXL" -slip 50 -trail 2.5

package main

import (
	"dropbear/broker/alpaca"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/loggy"
	"flag"
	"log"
	"strings"
)

var (
	flagSymbols   = flag.String("symbol", "FNGU", "symbols to monitor")
	flagBenchmark = flag.String("benchmark", "QQQ", "benchmark symbol")
	flagCash      = decimal.Flag("cash", "100_000", "initial USD balance")
	flagLookback  = flag.Int("lookback", 10, "breakout lookback period (minutes)")
	flagSlip      = decimal.FlagBPS("slip", "50", "max slippage willing to pay in basis points")
	flagMinPrice  = decimal.Flag("minprice", "10", "minimum share price to trade")
	flagMaxDD     = decimal.FlagPercent("maxdd", "5", "max daily drawdown before halting")
	flagMaxPos    = decimal.FlagPercent("maxpos", "60", "max position size as percent of portfolio")
	flagVerbose   = flag.Bool("v", false, "verbose logging")
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
	equity  *cubby.Equity
	bars    []alpaca.Bar
	dayHigh decimal.Decimal
	dayLow  decimal.Decimal
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
			log.Fatalf("paramgen tool wasn't run for symbol %s", sym)
		}
		equity, err := cubby.AddEquity(sym)
		if err != nil {
			log.Printf("error adding symbol %s: %v", sym, err)
			continue
		}
		state := &symbolState{equity: equity}
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
	log.Printf("  Lookback: %d min", *flagLookback)
	log.Printf("  Slippage: %s bps", flagSlip.MulInt(10000).Format(0))

	cubby.Run()
}

func onBar(state *symbolState, c *alpaca.Bar) {
	date := c.Timestamp.DateInt()
	if date != gCurrentDate {
		onDayChange()
		gCurrentDate = date
	}

	// Update rolling bar window
	state.bars = append(state.bars, *c)
	if len(state.bars) > *flagLookback {
		state.bars = state.bars[1:]
	}

	// Update day high/low
	if state.dayHigh.IsZero() || c.High.Cmp(state.dayHigh) > 0 {
		state.dayHigh = c.High
	}
	if state.dayLow.IsZero() || c.Low.Cmp(state.dayLow) < 0 {
		state.dayLow = c.Low
	}

	if len(state.bars) < *flagLookback {
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
		if *flagVerbose {
			log.Printf("OPEN: portfolio value $%s", gDayStart.FormatThousand(2))
		}
	}

	// Check for max daily drawdown
	if !gHalted && !gDayStart.IsZero() {
		current := cubby.GetPortfolioValue()
		drawdown := gDayStart.Sub(current).Div(gDayStart)
		if drawdown.Cmp(*flagMaxDD) >= 0 {
			gHalted = true
			if *flagVerbose {
				log.Printf("HALTED: drawdown %.2f%% exceeds max %.2f%%",
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
	// Calculate lookback high (excluding current bar)
	var lookbackHigh decimal.Decimal
	for i := 0; i < len(state.bars)-1; i++ {
		if lookbackHigh.IsZero() || state.bars[i].High.Cmp(lookbackHigh) > 0 {
			lookbackHigh = state.bars[i].High
		}
	}
	if lookbackHigh.IsZero() {
		return
	}

	price := c.Close

	// Check for breakout: price above lookback high by minimum gap
	params := OptimalParams[state.equity.Symbol]
	breakoutThreshold := lookbackHigh.Mul(decimal.One.Add(params.MinGap))
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

	// Place IOC limit order
	order, err := state.equity.LimitOrder(maxQty, limitPrice, alpaca.TimeInForceIOC)
	if err != nil {
		if *flagVerbose {
			log.Printf("error placing buy order for %s: %v", state.equity.Symbol, err)
		}
		return
	}

	if order.FilledQuantity.IsZero() {
		if *flagVerbose {
			log.Printf("BUY order not filled for %s", state.equity.Symbol)
		}
		return
	}

	// Track position with trailing stop
	gPositions[state.equity.Symbol] = &position{
		equity:    state.equity,
		highSince: order.FilledPrice,
	}

	if *flagVerbose {
		log.Printf("BUY %s %s at $%s (limit $%s)",
			order.FilledQuantity, state.equity.Symbol,
			order.FilledPrice, limitPrice.Format(2))
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

	if *flagVerbose {
		if order.FilledQuantity.IsZero() {
			log.Printf("SELL %s failed: 0 filled (limit $%s, status %s)",
				equity.Symbol, limitPrice.Format(2), order.Status)
		} else {
			log.Printf("SELL %s %s at $%s (%s)",
				order.FilledQuantity.Abs(), equity.Symbol,
				order.FilledPrice, reason)
		}
	}

	// If fully closed, remove from positions
	if equity.Quantity.IsZero() {
		delete(gPositions, equity.Symbol)
	}
}

func onDayChange() {
	for _, state := range gSymbols {
		state.bars = nil
		state.dayHigh = decimal.Zero
		state.dayLow = decimal.Zero
	}
	gPositions = make(map[string]*position)
	gDayStart = decimal.Zero
	gHalted = false
}
