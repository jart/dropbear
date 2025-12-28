// Command fngu implements a day trading strategy for FNGU (3x FANG+ ETN).
//
// FNGU has 30% maintenance margin instead of proper 75%, enabling
// 40x effective day trading leverage (4x buying power / 30% × 3x).
//
// Strategy: Intraday momentum breakout
//   - Enter long when price breaks above N-minute high
//   - Use trailing stop for exits
//   - Close all positions 15 minutes before market close
//   - Never hold overnight (avoid gap risk and decay)
//
// Usage:
//
//	go run ./cmd/fngu -backtest              # backtest mode
//	go run ./cmd/fngu -backtest -lookback 10 # 10-minute breakout
//	go run ./cmd/fngu -paper                 # paper trading
package main

import (
	"dropbear/clocky"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"flag"
	"fmt"
	"log"
	"time"
)

var (
	flagSymbol    = flag.String("symbol", "FNGU", "symbol to trade")
	flagBenchmark = flag.String("benchmark", "QQQ", "benchmark symbol")
	flagCash      = decimal.Flag("cash", "100_000", "initial USD balance")
	flagLookback  = flag.Int("lookback", 15, "breakout lookback period (minutes)")
	flagTrailPct  = decimal.Flag("trail", "0.025", "trailing stop percentage (0.025 = 2.5%)")
	flagMinGap    = decimal.Flag("mingap", "0.01", "minimum gap to enter (1%)")
	flagVerbose   = flag.Bool("v", false, "verbose logging")
)

// Margin multiplier - use cubby's -margin flag (default 1, use 4 for PDT day trading)
func getMargin() int {
	// Look up cubby's margin flag
	f := flag.Lookup("margin")
	if f == nil {
		return 1
	}
	if v, ok := f.Value.(flag.Getter); ok {
		return v.Get().(int)
	}
	return 1
}

var (
	gExchange  *cubby.Exchange
	gEquity    *cubby.Equity
	gBenchmark *cubby.Equity

	// Intraday state (reset each day)
	gCandles     []indicators.Candle // rolling window of recent candles
	gDayHigh     decimal.Decimal     // highest price seen today
	gDayLow      decimal.Decimal     // lowest price seen today
	gEntryPrice  decimal.Decimal     // price we entered at (zero if flat)
	gHighSince   decimal.Decimal     // highest price since entry (for trailing stop)
	gCurrentDate time.Time           // current trading day
	gTradesToday int                 // number of trades today
	gClosedToday bool                // already closed for the day

	// Statistics
	gTotalTrades   int
	gWinningTrades int
	gLosingTrades  int
	gTotalPnL      decimal.Decimal
)

const (
	marketOpenHour    = 9
	marketOpenMinute  = 30
	marketCloseHour   = 16
	marketCloseMinute = 0
	closeBeforeMin    = 15 // close positions this many minutes before market close
)

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	// Set up exchange and equities
	gExchange = cubby.Exchanges.Get(ds.ExchangeAlpaca)
	gEquity = gExchange.Equities.Get(*flagSymbol)
	gBenchmark = gExchange.Equities.Get(*flagBenchmark)

	// Set callbacks
	gEquity.OnCandle = onCandle
	gBenchmark.OnCandle = func(*indicators.Candle) {} // just for benchmark tracking

	// Set initial balance and benchmark
	cubby.SetBalance(ds.ExchangeAlpaca, "USD", *flagCash)
	cubby.SetBenchmark(gBenchmark)

	// Set up ready callback
	cubby.Exchanges.OnReady = onReady

	log.Printf("FNGU Day Trading Strategy")
	log.Printf("  Symbol: %s", *flagSymbol)
	log.Printf("  Lookback: %d minutes", *flagLookback)
	log.Printf("  Trail Stop: %.1f%%", flagTrailPct.Float64()*100)
	log.Printf("  Min Gap: %.2f%%", flagMinGap.Float64()*100)

	// Run the framework
	cubby.Run()

	// Print final stats
	printStats()
}

func onReady() {
	log.Printf("Market data ready, starting strategy...")
}

func onCandle(c *indicators.Candle) {
	now := time.UnixMicro(int64(c.Start))
	loc, _ := time.LoadLocation("America/New_York")
	now = now.In(loc)

	// Check for new trading day
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if !today.Equal(gCurrentDate) {
		resetDay(today)
	}

	// Skip pre-market and after-hours
	if !isMarketHours(now) {
		return
	}

	// Update rolling candle window
	gCandles = append(gCandles, *c)
	if len(gCandles) > *flagLookback {
		gCandles = gCandles[1:]
	}

	// Update day high/low
	if gDayHigh.IsZero() || c.High.Cmp(gDayHigh) > 0 {
		gDayHigh = c.High
	}
	if gDayLow.IsZero() || c.Low.Cmp(gDayLow) < 0 {
		gDayLow = c.Low
	}

	// Check if we need to close before market close
	closeTime := time.Date(now.Year(), now.Month(), now.Day(),
		marketCloseHour, marketCloseMinute-closeBeforeMin, 0, 0, loc)
	if now.After(closeTime) || now.Equal(closeTime) {
		closePosition("EOD")
		gClosedToday = true
		return
	}

	// Skip if already closed for the day
	if gClosedToday {
		return
	}

	// Need at least lookback candles before trading
	if len(gCandles) < *flagLookback {
		return
	}

	// Skip first 15 minutes of trading (let market settle)
	openTime := time.Date(now.Year(), now.Month(), now.Day(),
		marketOpenHour, marketOpenMinute+15, 0, 0, loc)
	if now.Before(openTime) {
		return
	}

	// Get current position
	shares := gEquity.Shares.Quantity.Load()
	hasPosition := shares.IsPositive()
	price := c.Close

	if hasPosition {
		// Update trailing stop
		if price.Cmp(gHighSince) > 0 {
			gHighSince = price
		}

		// Check trailing stop
		stopPrice := gHighSince.Mul(decimal.One.Sub(*flagTrailPct))
		if price.Cmp(stopPrice) <= 0 {
			closePosition("TRAIL_STOP")
		}
	} else {
		// Look for breakout entry
		checkBreakoutEntry(c, now)
	}
}

func checkBreakoutEntry(c *indicators.Candle, now time.Time) {
	if len(gCandles) < *flagLookback {
		return
	}

	// Calculate lookback high (excluding current candle)
	var lookbackHigh decimal.Decimal
	for i := 0; i < len(gCandles)-1; i++ {
		if lookbackHigh.IsZero() || gCandles[i].High.Cmp(lookbackHigh) > 0 {
			lookbackHigh = gCandles[i].High
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

	// Don't enter if we've already traded too much today (limit churn)
	if gTradesToday >= 3 {
		return
	}

	// Calculate position size: use full buying power (margin handled by cubby -margin flag)
	// PDT rules allow 4x leverage intraday, must close by EOD
	cash := gEquity.Cash.Available.Load()
	buyingPower := cash.MulInt(getMargin())
	// Reserve 5% buffer for slippage
	usableBuyingPower := buyingPower.MulInt(95).DivInt(100)
	qty := usableBuyingPower.Div(price).Int()

	if qty <= 0 {
		return
	}

	// Cap at 50,000 shares to avoid decimal overflow in fee calculations
	if qty > 50000 {
		qty = 50000
	}

	// Enter long position
	gEquity.MarketOrder(ds.SideBuy, qty)
	gEntryPrice = price
	gHighSince = price
	gTradesToday++
	gTotalTrades++

	if *flagVerbose {
		log.Printf("[%s] BUY %d @ $%s (breakout above $%s)",
			now.Format("15:04"), qty, price.Format(2), lookbackHigh.Format(2))
	}
}

func closePosition(reason string) {
	shares := gEquity.Shares.Quantity.Load()
	if !shares.IsPositive() {
		return
	}

	qty := shares.Int()
	if qty <= 0 {
		return
	}

	price := gEquity.LastPrice.Load()
	gEquity.MarketOrder(ds.SideSell, qty)

	// Calculate P&L
	if !gEntryPrice.IsZero() {
		pnl := price.Sub(gEntryPrice).Mul(shares)
		gTotalPnL = gTotalPnL.Add(pnl)

		if pnl.IsPositive() {
			gWinningTrades++
		} else {
			gLosingTrades++
		}

		if *flagVerbose {
			now := time.UnixMicro(int64(clocky.Now()))
			loc, _ := time.LoadLocation("America/New_York")
			now = now.In(loc)
			pnlPct := pnl.Div(gEntryPrice.Mul(shares)).MulInt(100)
			log.Printf("[%s] SELL %d @ $%s (%s) P&L: $%s (%.2f%%)",
				now.Format("15:04"), qty, price.Format(2), reason,
				pnl.Format(2), pnlPct.Float64())
		}
	}

	gEntryPrice = decimal.Zero
	gHighSince = decimal.Zero
}

func resetDay(today time.Time) {
	// Close any position from previous day (shouldn't happen, but safety)
	if gEquity != nil {
		shares := gEquity.Shares.Quantity.Load()
		if shares.IsPositive() {
			closePosition("NEW_DAY")
		}
	}

	gCurrentDate = today
	gCandles = nil
	gDayHigh = decimal.Zero
	gDayLow = decimal.Zero
	gEntryPrice = decimal.Zero
	gHighSince = decimal.Zero
	gTradesToday = 0
	gClosedToday = false

	if *flagVerbose {
		log.Printf("=== New trading day: %s ===", today.Format("2006-01-02"))
	}
}

func isMarketHours(t time.Time) bool {
	hour := t.Hour()
	minute := t.Minute()
	timeOfDay := hour*60 + minute

	marketOpen := marketOpenHour*60 + marketOpenMinute
	marketClose := marketCloseHour*60 + marketCloseMinute

	return timeOfDay >= marketOpen && timeOfDay < marketClose
}

func printStats() {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("FNGU Day Trading Strategy Results")
	fmt.Println("========================================")
	fmt.Printf("Total Trades: %d\n", gTotalTrades)
	fmt.Printf("Winning: %d\n", gWinningTrades)
	fmt.Printf("Losing: %d\n", gLosingTrades)
	if gTotalTrades > 0 {
		winRate := float64(gWinningTrades) / float64(gTotalTrades) * 100
		fmt.Printf("Win Rate: %.1f%%\n", winRate)
	}
	fmt.Printf("Total P&L (before fees): $%s\n", gTotalPnL.Format(2))
}
