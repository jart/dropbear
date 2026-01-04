// Command fngu implements a day trading strategy for FNGU (3x FANG+ ETN).
//
// Strategy: Intraday momentum breakout scanner
//   - Monitor multiple symbols for breakout patterns
//   - Enter long when price breaks above N-minute high
//   - Priority symbols can preempt lower-priority trades
//   - Use trailing stop for exits
//   - Close all positions 15 minutes before market close
//   - Hold a safe asset overnight (optional)
//
// Usage:
//
//	go run ./cmd/fngu -backtest -symbol "FNGU TQQQ SOXL"
//	go run ./cmd/fngu -backtest -symbol "$(cat symbols.txt)" -priority "FNGU TQQQ"

package main

import (
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/indicators"
	"dropbear/loggy"
	"flag"
	"log"
	"strings"
)

var (
	flagSymbols   = flag.String("symbol", "FNGU", "symbols to monitor (space-separated)")
	flagPriority  = flag.String("priority", "", "priority symbols (space-separated, earlier = higher)")
	flagBenchmark = flag.String("benchmark", "QQQ", "benchmark symbol")
	flagHodl      = flag.String("hodl", "", "symbol to hold overnight")
	flagCash      = decimal.Flag("cash", "100_000", "initial USD balance")
	flagLookback  = flag.Int("lookback", 15, "breakout lookback period (minutes)")
	flagBuffer    = decimal.FlagPercent("buffer", "0.5", "buffer percentage")
	flagTrail     = decimal.FlagPercent("trail", "2.5", "trailing stop percentage (2.5%)")
	flagMinGap    = decimal.FlagPercent("mingap", "1", "minimum gap to enter (1%)")
	flagVerbose   = flag.Bool("v", false, "verbose logging")
)

const (
	openTime  = 6_45_00
	closeTime = 12_45_00
)

// symbolState tracks per-symbol state for the scanner
type symbolState struct {
	equity   *cubby.Equity
	candles  []indicators.Candle
	dayHigh  decimal.Decimal
	dayLow   decimal.Decimal
	priority int // lower = higher priority; 999999 = not in priority list
}

var (
	gSymbols       map[string]*symbolState
	gHodl          *cubby.Equity
	gBenchmark     *cubby.Equity
	gCurrentEquity *cubby.Equity   // equity we're currently day trading (nil if not trading)
	gHighSince     decimal.Decimal // highest price since entry (for trailing stop)
	gTradesToday   int             // number of trades executed today
	gCurrentDate   int             // current trading date
	gIsOpen        bool            // market is open
)

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	// Parse symbols
	symbols := strings.Fields(*flagSymbols)
	if len(symbols) == 0 {
		log.Fatal("no symbols specified")
	}

	// Parse priority list and build priority map
	priorityList := strings.Fields(*flagPriority)
	priorityMap := make(map[string]int)
	for i, sym := range priorityList {
		priorityMap[sym] = i
	}

	// Create equity for each symbol
	gSymbols = make(map[string]*symbolState)
	for _, sym := range symbols {
		equity, err := cubby.AddEquity(sym)
		if err != nil {
			log.Printf("error adding symbol %s: %v", sym, err)
			continue
		}
		priority := 999999 // default: lowest priority
		if p, ok := priorityMap[sym]; ok {
			priority = p
		}
		state := &symbolState{
			equity:   equity,
			priority: priority,
		}
		gSymbols[sym] = state
		// Capture state in closure for callback
		equity.OnCandle = func(s *symbolState) func(*indicators.Candle) {
			return func(c *indicators.Candle) {
				onCandle(s, c)
			}
		}(state)
	}

	// Add benchmark
	gBenchmark, _ = cubby.AddEquity(*flagBenchmark)
	cubby.Benchmark = gBenchmark

	// Add hodl symbol if specified
	if *flagHodl != "" {
		gHodl, _ = cubby.AddEquity(*flagHodl)
	}

	cubby.Cash = *flagCash

	log.Printf("Multi-Symbol Day Trading Strategy")
	log.Printf("  Symbols: %d total", len(symbols))
	if len(priorityList) > 0 {
		log.Printf("  Priority: %s", strings.Join(priorityList, " > "))
	}
	if *flagHodl != "" {
		log.Printf("  Hodl: %s (overnight)", *flagHodl)
	}
	log.Printf("  Lookback: %d minutes", *flagLookback)
	log.Printf("  Trail Stop: %s%%", flagTrail.MulInt(100).Format(1))
	log.Printf("  Min Gap: %s%%", flagMinGap.MulInt(100).Format(1))

	cubby.Run()
}

func onCandle(state *symbolState, c *indicators.Candle) {
	// Check for day change (only need to do this once per timestamp)
	date := c.Start.DateInt()
	if date != gCurrentDate {
		onDayChange()
		gCurrentDate = date
	}

	// Update this symbol's rolling candle window
	state.candles = append(state.candles, *c)
	if len(state.candles) > *flagLookback {
		state.candles = state.candles[1:]
	}

	// Update day high/low
	if state.dayHigh.IsZero() || c.High.Cmp(state.dayHigh) > 0 {
		state.dayHigh = c.High
	}
	if state.dayLow.IsZero() || c.Low.Cmp(state.dayLow) < 0 {
		state.dayLow = c.Low
	}

	// Need at least lookback candles before trading
	if len(state.candles) < *flagLookback {
		return
	}

	// Only proceed if market is open
	time := c.Start.ClockInt()
	if !gIsOpen {
		if time >= openTime && time < closeTime {
			gIsOpen = true
			onMarketOpen()
		} else {
			return
		}
	} else {
		if time >= closeTime {
			onMarketClose()
			gIsOpen = false
			return
		}
	}

	// If we're currently trading this symbol, check trailing stop
	if gCurrentEquity == state.equity {
		price := c.Close
		if price.Cmp(gHighSince) > 0 {
			gHighSince = price
		}
		stopPrice := gHighSince.Mul(decimal.One.Sub(*flagTrail))
		if price.Cmp(stopPrice) <= 0 {
			closePosition(state.equity, "TRAIL_STOP")
			gCurrentEquity = nil
		}
		return
	}

	// Check for breakout entry opportunity
	checkBreakoutEntry(state, c)
}

func checkBreakoutEntry(state *symbolState, c *indicators.Candle) {
	// Calculate lookback high (excluding current candle)
	var lookbackHigh decimal.Decimal
	for i := 0; i < len(state.candles)-1; i++ {
		if lookbackHigh.IsZero() || state.candles[i].High.Cmp(lookbackHigh) > 0 {
			lookbackHigh = state.candles[i].High
		}
	}
	if lookbackHigh.IsZero() {
		return
	}

	price := c.Close

	// Check for breakout: price above lookback high by minimum gap
	breakoutThreshold := lookbackHigh.Mul(decimal.One.Add(*flagMinGap))
	if price.Cmp(breakoutThreshold) <= 0 {
		return
	}

	// Don't enter if we've already traded too much today
	if gTradesToday >= 3 {
		return
	}

	// If currently trading another symbol, check priority
	if gCurrentEquity != nil {
		currentState := gSymbols[gCurrentEquity.Symbol]
		// Only switch if new symbol has strictly higher priority (lower number)
		if state.priority >= currentState.priority {
			return
		}
		// Higher priority breakout - dump current position
		closePosition(gCurrentEquity, "PRIORITY")
		gCurrentEquity = nil
	}

	// Free up buying power from hodl position
	closePosition(gHodl, "FREE_CASH")

	// Calculate position size
	buyingPower := state.equity.GetMaxOrderQuantity()
	usableBuyingPower := buyingPower.Mul(decimal.One.Sub(*flagBuffer))
	quantity := usableBuyingPower.Truncate()
	if !quantity.IsPositive() {
		return
	}

	// Enter long position
	order, err := state.equity.MarketOrder(quantity)
	if err != nil {
		log.Printf("error placing buy order: %v", err)
		return
	}
	order.Wait()
	gHighSince = order.FilledPrice
	gTradesToday++
	gCurrentEquity = state.equity

	if *flagVerbose {
		log.Printf("BUY $%s of %s %s at $%s",
			order.FilledQuantity.Mul(order.FilledPrice).FormatThousand(2),
			order.FilledQuantity.Abs(), state.equity.Symbol, order.FilledPrice)
	}
}

func closePosition(equity *cubby.Equity, reason string) {
	if equity == nil {
		return
	}
	shares := equity.Quantity.Load()
	if shares.IsZero() {
		return
	}
	order, err := equity.MarketOrder(shares.Neg())
	if err != nil {
		log.Printf("error placing sell order: %v", err)
		return
	}
	order.Wait()
	if *flagVerbose {
		log.Printf("SELL $%s of %s %s at $%s (%s)",
			order.FilledQuantity.Neg().Mul(order.FilledPrice).FormatThousand(2),
			order.FilledQuantity.Abs(), equity.Symbol, order.FilledPrice, reason)
	}
	gHighSince = decimal.Zero
}

func onMarketOpen() {
}

func onMarketClose() {
	// Close any open day trade position
	if gCurrentEquity != nil {
		closePosition(gCurrentEquity, "EOD")
		gCurrentEquity = nil
	}
	// Buy hodl position for overnight
	buyHodl()
	// Reset intraday state for all symbols
	for _, state := range gSymbols {
		state.candles = nil
		state.dayHigh = decimal.Zero
		state.dayLow = decimal.Zero
	}
	gHighSince = decimal.Zero
}

func onDayChange() {
	// Reset all per-symbol state
	for _, state := range gSymbols {
		state.candles = nil
		state.dayHigh = decimal.Zero
		state.dayLow = decimal.Zero
	}
	gHighSince = decimal.Zero
	gTradesToday = 0
}

func buyHodl() {
	if gHodl == nil {
		return
	}
	price := gHodl.AskPrice.Load()
	if price.IsZero() {
		return
	}
	cash := cubby.Cash.Load()
	if !cash.IsPositive() {
		return
	}
	qty := cash.Div(price).Truncate()
	if !qty.IsPositive() {
		return
	}
	order, err := gHodl.MarketOrder(qty)
	if err != nil {
		log.Printf("error placing buy order: %v", err)
		return
	}
	order.Wait()
	if *flagVerbose {
		log.Printf("BUY $%s of %s %s at $%s",
			order.FilledQuantity.Mul(order.FilledPrice).FormatThousand(2),
			order.FilledQuantity.Abs(), gHodl.Symbol, order.FilledPrice)
	}
}
