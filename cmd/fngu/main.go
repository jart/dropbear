// Command fngu implements a day trading strategy for FNGU (3x FANG+ ETN).
//
// Leverage breakdown:
//   - Reg-T initial margin: 50% = 2x overnight buying power
//   - PDT intraday margin: 25% = 4x day trading buying power
//   - FNGU is 3x leveraged ETN
//   - Effective leverage: 4x (PDT) × 3x (ETN) = 12x intraday
//
// The 30% maintenance margin (vs 75% for normal leveraged ETFs) just means
// less chance of margin calls overnight, not more buying power.
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
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"flag"
	"log"
)

var (
	flagSymbol    = flag.String("symbol", "FNGU", "symbol to trade")
	flagBenchmark = flag.String("benchmark", "QQQ", "benchmark symbol")
	flagHodl      = flag.String("hodl", "", "symbol to hold when not trading")
	flagCash      = decimal.Flag("cash", "100_000", "initial USD balance")
	flagLookback  = flag.Int("lookback", 15, "breakout lookback period (minutes)")
	flagBuffer    = decimal.FlagPercent("buffer", "10", "buffer percentage (10%)")
	flagTrail     = decimal.FlagPercent("trail", "2.5", "trailing stop percentage (2.5%)")
	flagMinGap    = decimal.FlagPercent("mingap", "1", "minimum gap to enter (1%)")
	flagVerbose   = flag.Bool("v", false, "verbose logging")
)

const (
	openTime  = 6_30_00 + 15_00
	closeTime = 13_00_00 - 15_00
)

var (
	gEquity    *cubby.Equity
	gHodl      *cubby.Equity
	gBenchmark *cubby.Equity

	// Intraday state (reset each day)
	gCandles     []indicators.Candle // rolling window of recent candles
	gDayHigh     decimal.Decimal     // highest price seen today
	gDayLow      decimal.Decimal     // lowest price seen today
	gHighSince   decimal.Decimal     // highest price since entry (for trailing stop)
	gTradesToday int                 // number of trades executed today
	gCurrentDate int                 // current trading date
	gIsOpen      bool                // market is open
)

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	gEquity = cubby.AddEquity(*flagSymbol)
	gBenchmark = cubby.AddEquity(*flagBenchmark)
	if *flagHodl != "" {
		gHodl = cubby.AddEquity(*flagHodl)
	}
	gEquity.OnCandle = onCandle
	cubby.Benchmark = gBenchmark
	cubby.Cash = *flagCash

	log.Printf("FNGU Day Trading Strategy")
	log.Printf("  Symbol: %s", *flagSymbol)
	if *flagHodl != "" {
		log.Printf("  Hodl: %s (overnight yield)", *flagHodl)
	}
	log.Printf("  Lookback: %d minutes", *flagLookback)
	log.Printf("  Trail Stop: %s%%", flagTrail.MulInt(100).Format(1))
	log.Printf("  Min Gap: %s%%", flagBuffer.MulInt(100).Format(1))

	cubby.Run()
}

func onCandle(c *indicators.Candle) {

	// check for day change
	date := c.Start.DateInt()
	if date != gCurrentDate {
		onDayChange()
		gCurrentDate = date
	}

	// update rolling candle window
	gCandles = append(gCandles, *c)
	if len(gCandles) > *flagLookback {
		gCandles = gCandles[1:]
	}

	// update day high/low
	if gDayHigh.IsZero() || c.High.Cmp(gDayHigh) > 0 {
		gDayHigh = c.High
	}
	if gDayLow.IsZero() || c.Low.Cmp(gDayLow) < 0 {
		gDayLow = c.Low
	}

	// need at least lookback candles before trading
	if len(gCandles) < *flagLookback {
		return
	}

	// only proceed if market is open
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

	// Get current position
	shares := gEquity.Quantity.Load()
	invested := !shares.IsZero()
	price := c.Close

	if invested {
		// update trailing stop
		if price.Cmp(gHighSince) > 0 {
			gHighSince = price
		}
		// check trailing stop
		stopPrice := gHighSince.Mul(decimal.One.Sub(*flagTrail))
		if price.Cmp(stopPrice) <= 0 {
			close(gEquity, "TRAIL_STOP")
		}
	} else {
		checkBreakoutEntry(c)
	}
}

func checkBreakoutEntry(c *indicators.Candle) {

	// calculate lookback high (excluding current candle)
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

	// check for breakout: price above lookback high by minimum gap
	breakoutThreshold := lookbackHigh.Mul(decimal.One.Add(*flagMinGap))
	if price.Cmp(breakoutThreshold) <= 0 {
		return
	}

	// don't enter if we've already traded too much today (limit churn)
	if gTradesToday >= 3 {
		return
	}

	// free up buying power
	close(gHodl, "FREE_CASH")

	// Calculate position size: use full buying power (margin handled by cubby -margin flag)
	// PDT rules allow 4x leverage intraday, must close by EOD
	// Cap max position to $1B to avoid decimal overflow on sale proceeds
	// (with 40x leverage and big gains, selling can produce multi-billion proceeds)
	buyingPower := gEquity.GetBuyingPower()
	usableBuyingPower := buyingPower.Mul(decimal.One.Sub(*flagBuffer))
	qty := usableBuyingPower.Div(price).Truncate()
	if !qty.IsPositive() {
		return
	}

	// enter long position
	order := gEquity.MarketOrder(qty)
	order.Wait()
	gHighSince = price
	gTradesToday++

	if *flagVerbose {
		log.Printf("BUY %s of %s at %s", qty.Mul(price), *flagSymbol, price)
	}
}

func close(equity *cubby.Equity, reason string) {
	shares := equity.Quantity.Load()
	if shares.IsZero() {
		return
	}
	quantity := shares.Neg()
	price := equity.LastPrice.Load()
	equity.MarketOrder(quantity).Wait()
	if !equity.EntryPrice.IsZero() {
		pnl := price.Sub(equity.EntryPrice).Mul(shares)
		if *flagVerbose {
			pnlPct := pnl.Div(equity.EntryPrice.Mul(shares)).MulInt(100)
			log.Printf("SELL %s of %s at %s P&L: $%s (%s%%)",
				quantity.Mul(price), equity.Symbol, price, price.Format(2), reason,
				pnl.Format(2), pnlPct.Format(2))
		}
	}
	gHighSince = decimal.Zero
}

func onMarketOpen() {
}

func onMarketClose() {
	close(gEquity, "EOD")
	buyHodl()
	gCandles = nil
	gDayHigh = decimal.Zero
	gDayLow = decimal.Zero
	gHighSince = decimal.Zero
}

func onDayChange() {
	gCandles = nil
	gDayHigh = decimal.Zero
	gDayLow = decimal.Zero
	gHighSince = decimal.Zero
	gTradesToday = 0
}

func buyHodl() {
	if gHodl != nil {
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
		gHodl.MarketOrder(ds.SideBuy, qty)
		if *flagVerbose {
			log.Printf("BUY %s of %s at %s", qty.Mul(gHodl.AskPrice), *flagSymbol, price)
		}
	}
}
