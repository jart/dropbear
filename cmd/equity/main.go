// Command equity is an example equities trading strategy using cubby.
// Uses SMA trend following - buys when price is above N-day SMA, sells when below.
//
// Usage:
//
//	go run ./cmd/equity -backtest -symbol AAPL
//	go run ./cmd/equity -backtest -symbol AAPL -start 2020-01-01 -end 2020-12-31
package main

import (
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"flag"
)

var (
	flagSymbol    = flag.String("symbol", "AAPL", "stock symbol to trade")
	flagBenchmark = flag.String("benchmark", "SPY", "benchmark symbol")
	flagCash      = decimal.Flag("cash", "100_000", "initial USD balance")
	flagPeriod    = flag.Int("period", 200, "SMA period in trading days")
)

var (
	gBroker    *cubby.Broker
	gEquity    *cubby.Equity
	gBenchmark *cubby.Equity
	gSMA       *indicators.SMA
)

// Minutes per trading day
const minutesPerDay = 390

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	// Set up broker and equities
	gBroker = cubby.Brokers.Get(ds.BrokerAlpaca)
	gEquity = gBroker.Equities.Get(*flagSymbol)
	gBenchmark = gBroker.Equities.Get(*flagBenchmark)

	// Set initial balance and benchmark
	cubby.SetBalance(ds.BrokerAlpaca, "USD", *flagCash)
	cubby.SetBenchmark(gBenchmark)

	// Initialize SMA indicator (convert days to minutes)
	gSMA = indicators.NewSMA(*flagPeriod * minutesPerDay)

	// Register callbacks
	gEquity.OnReady = onReady
	gEquity.OnCandle = onCandle

	// Run the framework
	cubby.Run()
}

func onReady() {
	// Strategy is ready to start
}

func onCandle(c *indicators.Candle) {
	price := c.Close

	// Update SMA indicator
	gSMA.Add(price)

	// Wait until indicator is ready
	if !gSMA.IsReady() {
		return
	}

	// Trend signal: bullish when price is above SMA
	smaValue := gSMA.Value()
	bullish := price.Cmp(smaValue) > 0

	// Current position
	shares := gEquity.Shares.Quantity.Load()
	hasPosition := shares.IsPositive()
	buyingPower := gEquity.Broker.DayTradingBuyingPower.Load()

	switch {
	case bullish && !hasPosition && buyingPower.IsPositive():
		// Price above SMA, no position - buy with 95% of available buying power
		investAmount := buyingPower.Mul(decimal.Parse("0.95"))
		if price.IsPositive() {
			qty := investAmount.Div(price).Int()
			if qty > 0 {
				gEquity.MarketOrder(ds.SideBuy, qty)
			}
		}

	case !bullish && hasPosition:
		// Price below SMA, have position - sell all
		qty := shares.Int()
		if qty > 0 {
			gEquity.MarketOrder(ds.SideSell, qty)
		}
	}
}
