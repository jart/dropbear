//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|             trading algorithm x3.101-2026

// dayo is a simple day trading strategy that buys at open and sells before close.
//
// Usage:
//
//	go run ./cmd/dayo -symbol "GOOG PM" -backtest -start 2024-01-01
//
// At market open, it invests all buying power equally into the specified symbols.
// Two minutes before market close, it liquidates all positions.
package main

import (
	"dropbear/clocky"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"dropbear/loggy"
	"flag"
	"log"
)

var (
	flagSymbols   = flag.String("symbol", "", "symbols to trade (space-separated)")
	flagBenchmark = flag.String("benchmark", "SPY", "benchmark symbol")
	flagSlippage  = decimal.FlagBPS("slip", "25", "max slippage on orders")
)

const (
	openTime       = 6_30_00  // 6:30 AM PT - market open
	startCloseTime = 12_58_00 // 12:58 PM PT - start closing (2 min before close)
	closeTime      = 13_00_00 // 1:00 PM PT - market close
)

var (
	gTraders []*Trader
	gDate    int
	gOpened  bool // true if we've opened positions today
	gClosing bool // true if we've started closing positions today
)

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	if *flagSymbols == "" {
		log.Fatal("usage: dayo -symbol \"GOOG PM\" [-backtest] [-start DATE]")
	}

	syms, err := symbol.Expand(*flagSymbols)
	if err != nil {
		log.Fatalf("invalid symbols: %v", err)
	}
	if len(syms) == 0 {
		log.Fatal("no symbols specified")
	}

	for _, sym := range syms {
		equity, err := cubby.AddEquity(sym)
		if err != nil {
			log.Printf("error adding symbol %s: %v", sym, err)
			continue
		}
		trader := &Trader{equity: equity}
		gTraders = append(gTraders, trader)
		equity.OnBar = trader.onBar
	}

	if len(gTraders) == 0 {
		log.Fatal("no valid symbols to trade")
	}

	benchSym := symbol.MustParse(*flagBenchmark)
	benchEquity, _ := cubby.AddEquity(benchSym)
	cubby.Benchmark = benchEquity

	log.Printf("Day Trading Strategy")
	log.Printf("  Symbols: %d", len(gTraders))
	for _, t := range gTraders {
		log.Printf("    %s", t.equity.Symbol)
	}

	cubby.Run()
}

type Trader struct {
	equity        *cubby.Equity
	order         *cubby.Order
	monotonicTime clocky.Time
}

func (t *Trader) onBar(bar *ds.Bar) {

	// only process monotonically increasing time
	if !bar.Timestamp.After(t.monotonicTime) {
		return
	}
	t.monotonicTime = bar.Timestamp

	// check for new day
	date := bar.Timestamp.DateInt()
	if date != gDate {
		gDate = date
		gOpened = false
		gClosing = false
	}

	// skip if still warming up
	if cubby.IsWarmingUp {
		return
	}

	// check order status
	if t.order != nil && t.order.Status.IsFinal() {
		t.order = nil
	}

	now := clocky.Now()
	time := now.ClockInt()

	// before market open
	if time < openTime {
		return
	}

	// after market close
	if time >= closeTime {
		return
	}

	// time to close positions (2 min before close)
	if time >= startCloseTime {
		if !gClosing {
			gClosing = true
			if cubby.Verbose {
				log.Printf("starting end-of-day liquidation")
			}
		}
		t.closePosition()
		return
	}

	// time to open positions
	if !gOpened {
		gOpened = true
		if cubby.Verbose {
			log.Printf("opening positions at market open")
		}
	}

	// try to open position if we don't have one yet
	t.tryOpen()
}

func (t *Trader) tryOpen() {

	// already have a position
	if !t.equity.Quantity.IsZero() {
		return
	}

	// already have a pending order
	if t.order != nil && t.order.Status.IsOpen() {
		return
	}

	if !t.equity.Price.IsPositive() {
		return
	}

	// calculate limit price with slippage
	limitPrice := t.equity.Price.Mul(decimal.One.Add(*flagSlippage))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	// divide max order quantity equally among all symbols
	qty := t.equity.GetMaxOrderQuantity(limitPrice).DivInt(len(gTraders)).Truncate()

	if !qty.IsPositive() {
		return
	}

	order, err := t.equity.Order(qty, limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("error opening %s: %v", t.equity.Symbol, err)
		}
		return
	}
	t.order = order
}

func (t *Trader) closePosition() {
	shares := t.equity.Quantity
	if shares.IsZero() {
		return
	}

	// already have a closing order pending
	if t.order != nil && t.order.Status.IsOpen() {
		if t.order.Side == ds.SideSell && shares.IsPositive() {
			return // already selling
		}
		if t.order.Side == ds.SideBuy && shares.IsNegative() {
			return // already buying to cover
		}
	}

	// calculate limit price with slippage
	limitPrice := t.equity.Price
	if shares.IsPositive() {
		limitPrice = limitPrice.Mul(decimal.One.Sub(*flagSlippage))
	} else {
		limitPrice = limitPrice.Mul(decimal.One.Add(*flagSlippage))
	}
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	order, err := t.equity.Order(shares.Neg(), limitPrice)
	if err != nil {
		log.Printf("error closing %s: %v", t.equity.Symbol, err)
		return
	}
	t.order = order
}
