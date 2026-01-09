//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|             trading algorithm x3.101-2026

// dayo is a momentum-based day trading strategy.
//
// Usage:
//
//	go run ./cmd/dayo -backtest -start 2025-12-01
//
// It waits for positive momentum before entering, uses trailing stops,
// and liquidates before market close with adaptive pricing.
package main

import (
	"dropbear/clocky"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"dropbear/indicators"
	"dropbear/loggy"
	"flag"
	"log"
)

var (
	flagSymbols    = flag.String("symbol", "GOOG", "symbols to trade (space-separated)")
	flagSlipOpen   = decimal.FlagBPS("slip-open", "25", "slippage on open orders")
	flagSlipClose  = decimal.FlagBPS("slip-close", "50", "slippage on close orders")
	flagUrgency    = decimal.FlagBPS("urgency", "100", "extra slippage per minute when closing late")
	flagStopATR    = decimal.Flag("stop-atr", "1.5", "stop loss in ATR multiples")
	flagMomentum   = flag.Int("momentum", 7, "momentum lookback period in bars")
	flagWarmup     = flag.Int("warmup", 30, "warmup bars before trading")
	flagEntryStart = flag.Int("entry-start", 6_45_00, "earliest time to enter (HHMMSS)")
	flagEntryEnd   = flag.Int("entry-end", 11_00_00, "latest time to enter (HHMMSS)")
	flagBOP        = flag.Bool("bop", false, "enable BOP filter (skip entry when sellers in control)")
	flagNoMomentum = flag.Bool("no-momentum", false, "disable momentum filter (buy at open every day)")
)

const (
	openTime         = 6_30_00
	normalClose      = 13_00_00
	normalStartClose = 12_50_00
	earlyClose       = 10_00_00
	earlyStartClose  = 9_50_00
)

var (
	gTraders        []*Trader
	gDate           int
	gCloseTime      int
	gStartCloseTime int
	gOpened         bool
	gClosing        bool
)

var earlyCloseDates = map[int]bool{
	20240703: true, 20241129: true, 20241224: true,
	20250703: true, 20251128: true, 20251224: true,
	20260703: true, 20261127: true, 20261224: true,
}

func isEarlyCloseDay(date int) bool {
	return earlyCloseDates[date]
}

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	if *flagSymbols == "" {
		log.Fatal("usage: dayo -symbol \"GOOG AMZN\" [-backtest] [-start DATE]")
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
		trader := NewTrader(equity)
		gTraders = append(gTraders, trader)
		equity.OnBar = trader.onBar
	}

	if len(gTraders) == 0 {
		log.Fatal("no valid symbols to trade")
	}

	log.Printf("Momentum Day Trading Strategy")
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

	// indicators
	momentum *indicators.Momentum
	atr      *indicators.ATR
	bop      *indicators.BOP

	// position tracking
	entryPrice decimal.Decimal
	highSince  decimal.Decimal
	stopPrice  decimal.Decimal
	barCount   int
}

func NewTrader(equity *cubby.Equity) *Trader {
	return &Trader{
		equity:   equity,
		momentum: indicators.NewMomentum(*flagMomentum),
		atr:      indicators.NewATR(14),
		bop:      indicators.NewBOP(5),
	}
}

func (t *Trader) onBar(bar *ds.Bar) {
	if !bar.Timestamp.After(t.monotonicTime) {
		return
	}
	t.monotonicTime = bar.Timestamp

	// new day reset
	date := bar.Timestamp.DateInt()
	if date != gDate {
		gDate = date
		gOpened = false
		gClosing = false
		t.barCount = 0
		t.entryPrice = decimal.Zero
		t.highSince = decimal.Zero
		t.stopPrice = decimal.Zero
		if isEarlyCloseDay(date) {
			gCloseTime = earlyClose
			gStartCloseTime = earlyStartClose
		} else {
			gCloseTime = normalClose
			gStartCloseTime = normalStartClose
		}
	}

	if cubby.IsWarmingUp {
		return
	}

	// update indicators
	candle := &indicators.Candle{
		Start:  bar.Timestamp,
		Open:   bar.Open,
		High:   bar.High,
		Low:    bar.Low,
		Close:  bar.Close,
		Volume: bar.Volume,
	}
	t.momentum.Add(bar.Close)
	t.atr.Add(candle)
	t.bop.Add(candle)
	t.barCount++

	// check order status
	if t.order != nil && t.order.Status.IsFinal() {
		t.order = nil
	}

	// track high since entry for trailing stop
	if !t.equity.Quantity.IsZero() && bar.High.Cmp(t.highSince) > 0 {
		t.highSince = bar.High
		// update trailing stop
		if t.atr.IsReady() {
			t.stopPrice = t.highSince.Sub(t.atr.Value.Mul(*flagStopATR))
		}
	}

	now := clocky.Now()
	time := now.ClockInt()

	if time < openTime || time >= gCloseTime {
		return
	}

	// check stop loss
	if !t.equity.Quantity.IsZero() && t.stopPrice.IsPositive() {
		if bar.Low.Cmp(t.stopPrice) <= 0 {
			if cubby.Verbose {
				log.Printf("stop loss triggered for %s at %s (stop: %s)",
					t.equity.Symbol, bar.Close, t.stopPrice)
			}
			t.closePosition(time, gStartCloseTime, gCloseTime)
			return
		}
	}

	// time to close
	if time >= gStartCloseTime {
		if !gClosing {
			gClosing = true
			if cubby.Verbose {
				log.Printf("starting end-of-day liquidation")
			}
		}
		t.closePosition(time, gStartCloseTime, gCloseTime)
		return
	}

	// try to open
	if !gOpened {
		gOpened = true
	}
	t.tryOpen(time)
}

func (t *Trader) tryOpen(time int) {
	// already have position
	if !t.equity.Quantity.IsZero() {
		return
	}

	// already have pending order
	if t.order != nil && t.order.Status.IsOpen() {
		return
	}

	if !t.equity.Price.IsPositive() {
		return
	}

	// check entry window
	if time < *flagEntryStart || time > *flagEntryEnd {
		return
	}

	// wait for warmup
	if t.barCount < *flagWarmup {
		return
	}

	// check indicators are ready
	if !t.atr.IsReady() {
		return
	}

	if !*flagNoMomentum {
		if !t.momentum.IsReady() {
			return
		}
		// only enter on positive momentum
		if !t.momentum.Value.IsPositive() {
			return
		}
		// optional: check BOP for confirmation
		if *flagBOP && t.bop.IsReady() && t.bop.Value.IsNegative() {
			return // sellers in control, skip
		}
	}

	// calculate limit price
	limitPrice := t.equity.Price.Mul(decimal.One.Add(*flagSlipOpen))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	// divide equally among symbols
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
	t.entryPrice = t.equity.Price
	t.highSince = t.equity.Price
	if t.atr.IsReady() {
		t.stopPrice = t.equity.Price.Sub(t.atr.Value.Mul(*flagStopATR))
	}
}

func (t *Trader) closePosition(time, startClose, closeTime int) {
	shares := t.equity.Quantity
	if shares.IsZero() {
		return
	}

	// calculate urgency for adaptive slippage
	urgency := decimal.Zero
	if time >= startClose {
		totalWindow := closeTime - startClose
		elapsed := time - startClose
		if totalWindow > 0 {
			urgency = decimal.FromInt(elapsed).Div(decimal.FromInt(totalWindow))
		}
	}
	adaptiveSlip := flagSlipClose.Add(urgency.Mul(*flagUrgency))

	// check if we should cancel and replace stale order
	if t.order != nil && t.order.Status.IsOpen() {
		if t.order.Side == ds.SideSell && shares.IsPositive() {
			priceDiff := t.equity.Price.Sub(t.order.LimitPrice).Div(t.equity.Price).Abs()
			if priceDiff.Cmp(decimal.Parse("0.005")) > 0 {
				t.order.Cancel()
				t.order = nil
			} else {
				return
			}
		} else if t.order.Side == ds.SideBuy && shares.IsNegative() {
			return
		}
	}

	// calculate limit price
	limitPrice := t.equity.Price
	if shares.IsPositive() {
		limitPrice = limitPrice.Mul(decimal.One.Sub(adaptiveSlip))
	} else {
		limitPrice = limitPrice.Mul(decimal.One.Add(adaptiveSlip))
	}
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	order, err := t.equity.Order(shares.Neg(), limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("error closing %s: %v", t.equity.Symbol, err)
		}
		return
	}
	t.order = order
}
