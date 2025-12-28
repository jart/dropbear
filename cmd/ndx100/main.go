// Command ndx100 implements equal-weight rebalancing across NASDAQ-100 stocks.
// Divides capital equally and rebalances periodically to buy dips and sell rallies.
//
// Usage:
//
//	go run ./cmd/ndx100 -backtest
//	go run ./cmd/ndx100 -backtest -rebalance 5d
package main

import (
	"bufio"
	"dropbear/clocky"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"flag"
	"os"
	"path/filepath"
	"strings"
)

var (
	flagSymbols   = flag.String("symbols", "ndx100.txt", "file containing symbols to trade")
	flagBenchmark = flag.String("benchmark", "QQQ", "benchmark symbol")
	flagCash      = decimal.Flag("cash", "100_000", "initial USD balance")
	flagRebalance = clocky.DurationFlag("rebalance", "5d", "rebalance interval")
)

var (
	gExchange      *cubby.Exchange
	gBenchmark     *cubby.Equity
	gEquities      []*cubby.Equity
	gLastRebalance clocky.Time
)

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	// Load symbols from file
	symbols := loadSymbols(*flagSymbols)
	if len(symbols) == 0 {
		loggy.Fatalf("no symbols loaded from %s", *flagSymbols)
	}

	// Set up exchange and equities
	gExchange = cubby.Exchanges.Get(ds.ExchangeAlpaca)
	gBenchmark = gExchange.Equities.Get(*flagBenchmark)

	// Register all equities
	for _, sym := range symbols {
		eq := gExchange.Equities.Get(sym)
		eq.OnCandle = makeOnCandle(eq)
		gEquities = append(gEquities, eq)
	}

	// Set initial balance and benchmark
	cubby.SetBalance(ds.ExchangeAlpaca, "USD", *flagCash)
	cubby.SetBenchmark(gBenchmark)

	// Set up global ready callback for initial allocation
	cubby.Exchanges.OnReady = onReady

	// Run the framework
	cubby.Run()
}

func loadSymbols(filename string) []string {
	// Try relative to current dir first, then working dir
	paths := []string{
		filename,
		filepath.Join(os.Getenv("HOME"), "dropbear", filename),
	}

	var file *os.File
	var err error
	for _, p := range paths {
		file, err = os.Open(p)
		if err == nil {
			break
		}
	}
	if file == nil {
		loggy.Fatalf("could not open symbols file: %v", err)
	}
	defer file.Close()

	var symbols []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Split on whitespace to handle space-separated symbols
		for _, sym := range strings.Fields(line) {
			symbols = append(symbols, sym)
		}
	}
	return symbols
}

func onReady() {
	// Initial allocation - buy equal weight of all stocks
	rebalance()
}

func makeOnCandle(eq *cubby.Equity) func(*indicators.Candle) {
	return func(c *indicators.Candle) {
		// Check if it's time to rebalance (only check once per candle via first equity)
		if eq == gEquities[0] {
			now := clocky.Now()
			if gLastRebalance.IsZero() || now.Sub(gLastRebalance) >= *flagRebalance {
				rebalance()
				gLastRebalance = now
			}
		}
	}
}

func rebalance() {
	// Calculate total portfolio value
	totalEquity := cubby.GetEquityUSD()
	if !totalEquity.IsPositive() {
		return
	}

	// Target value per stock (equal weight)
	numStocks := len(gEquities)
	targetPerStock := totalEquity.DivInt(numStocks)

	// Threshold for trading (5% of target to reduce churn)
	threshold := targetPerStock.MulInt(5).DivInt(100)

	// First pass: execute all sells to free up cash
	for _, eq := range gEquities {
		price := eq.LastPrice.Load()
		if !price.IsPositive() {
			continue
		}

		shares := eq.Shares.Quantity.Load()
		currentValue := shares.Mul(price)
		diff := targetPerStock.Sub(currentValue)

		// Sell if overweight
		if diff.IsNegative() && diff.Abs().Cmp(threshold) >= 0 {
			sellValue := diff.Neg()
			sellQty := sellValue.Div(price).Int()
			currentQty := shares.Int()
			if sellQty > currentQty {
				sellQty = currentQty
			}
			if sellQty > 0 {
				eq.MarketOrder(ds.SideSell, sellQty)
			}
		}
	}

	// Second pass: execute buys with available cash
	for _, eq := range gEquities {
		price := eq.LastPrice.Load()
		if !price.IsPositive() {
			continue
		}

		shares := eq.Shares.Quantity.Load()
		currentValue := shares.Mul(price)
		diff := targetPerStock.Sub(currentValue)

		// Buy if underweight
		if diff.IsPositive() && diff.Cmp(threshold) >= 0 {
			// Read fresh cash each time (cubby decrements on order)
			availableCash := eq.Cash.Available.Load()
			// MarketOrder adds 5% buffer + fees, so use 10% margin
			costPerShare := price.MulInt(110).DivInt(100)
			if availableCash.Cmp(costPerShare) < 0 {
				continue
			}
			// Calculate max shares we can afford
			maxQty := availableCash.Div(costPerShare).Int()
			wantQty := diff.Div(price).Int()
			qty := wantQty
			if qty > maxQty {
				qty = maxQty
			}
			if qty > 0 {
				eq.MarketOrder(ds.SideBuy, qty)
			}
		}
	}
}
