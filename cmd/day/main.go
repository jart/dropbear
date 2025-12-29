// Command day demonstrates the scheduling framework by buying a stock
// after market open and selling before close.
//
// Usage:
//
//	go run ./cmd/day -backtest -symbol GOOG -margin 4
//	go run ./cmd/day -backtest -symbol GOOG -overnight -margin 2
package main

import (
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"flag"
)

var (
	flagSymbol    = flag.String("symbol", "TSLA", "stock symbol to day trade")
	flagCash      = decimal.Flag("cash", "100_000", "initial USD balance")
	flagOvernight = flag.Bool("overnight", false, "hold overnight only instead")
)

var (
	gExchange *cubby.Exchange
	gStock    *cubby.Equity
)

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	// Set up exchange and equity
	gExchange = cubby.Exchanges.Get(ds.ExchangeAlpaca)
	gStock = gExchange.Equities.Get(*flagSymbol)

	// Set initial balance
	cubby.SetBalance(ds.ExchangeAlpaca, "USD", *flagCash)
	cubby.SetBenchmark(gStock)

	// Register scheduling callbacks
	if *flagOvernight {
		cubby.BeforeClose(sell)
		cubby.AfterOpen(buy)
	} else {
		cubby.AfterOpen(buy)
		cubby.BeforeClose(sell)
	}

	// Run the framework
	cubby.Run()
}

func buy() {
	price := gStock.LastPrice.Load()
	if !price.IsPositive() {
		return
	}

	// Use all available buying power
	// Account for 10% buffer for price movement + fees
	buyingPower := gExchange.DayTradingBuyingPower.Load()
	costPerShare := price.Mul(decimal.Parse("1.1"))
	if buyingPower.Cmp(costPerShare) < 0 {
		return // not enough for even 1 share
	}

	qty := buyingPower.Div(costPerShare).Int()
	if qty > 0 {
		gStock.MarketOrder(ds.SideBuy, qty)
	}
}

func sell() {
	// Sell all shares
	shares := gStock.Shares.Quantity.Load().Int()
	if shares > 0 {
		gStock.MarketOrder(ds.SideSell, shares)
	}
}
