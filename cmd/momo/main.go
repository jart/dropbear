//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|             trading algorithm x3.105-2026

// momo implements a simple momentum strategy.
//
// Usage:
//
//	go run ./cmd/momo -backtest -start 2025-10-01 -symbol "GOOG"
//
// Strategy:
//   - Enter when price makes new N-bar high AND momentum is positive
//   - Trail stop based on ATR
//   - Close before market close
package main

import (
	"flag"
	"log"

	"dropbear/broker/alpaca"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"dropbear/symbol"
)

var (
	flagSymbols = flag.String("symbol", "GOOG", "symbols to trade")

	// Entry parameters
	flagLookback   = flag.Int("lookback", 20, "bars to look back for high")
	flagMomentum   = flag.Int("momentum", 10, "momentum calculation period")
	flagEntryStart = clocky.DurationFlag("entry-start", "30m", "earliest entry after open")
	flagEntryEnd   = clocky.DurationFlag("entry-end", "4h", "latest entry after open")

	// Exit parameters
	flagTrailATRMult = decimal.Flag("trail-atr-mult", "2", "ATR multiplier for trail stop")
	flagATRPeriod    = flag.Int("atr-period", 14, "ATR period")
	flagStopLoss     = decimal.FlagBPS("stop-loss", "100", "max loss from entry (bps)")

	// Risk
	flagPositionPct = decimal.Flag("position-pct", "0.5", "position size")
)

const orderStaleness = 2 * clocky.Minute

var gDate int

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	syms, err := symbol.Expand(*flagSymbols)
	if err != nil {
		log.Fatal(err)
	}

	for _, sym := range syms {
		equity, err := cubby.AddEquity(sym)
		if err != nil {
			log.Printf("error: %v", err)
			continue
		}
		t := &Trader{
			equity:   equity,
			atr:      indicators.NewATR(*flagATRPeriod),
			momentum: indicators.NewMomentum(*flagMomentum),
			maxPrice: indicators.NewMax(clocky.Duration(*flagLookback) * clocky.Minute),
		}
		equity.OnBar = t.onBar
	}

	log.Printf("Momentum Strategy: lookback=%d, momentum=%d, trail=%sx ATR",
		*flagLookback, *flagMomentum, *flagTrailATRMult)
	cubby.Run()
}

type Trader struct {
	equity   *cubby.Equity
	lastTime clocky.Time
	lastDate int

	atr      *indicators.ATR
	momentum *indicators.Momentum
	maxPrice *indicators.Max

	entryOrder *cubby.Order
	exitOrder  *cubby.Order
	entry      decimal.Decimal
	shares     decimal.Decimal
	peakPrice  decimal.Decimal
	stopPrice  decimal.Decimal
	dailyPnL   decimal.Decimal
}

func (t *Trader) onBar(bar *ds.Bar) {
	if !bar.Timestamp.After(t.lastTime) {
		return
	}
	t.lastTime = bar.Timestamp

	date := bar.Timestamp.DateInt()
	if date != gDate {
		gDate = date
	}
	if date != t.lastDate {
		t.lastDate = date
		t.dailyPnL = decimal.Zero
	}

	if cubby.IsWarmingUp {
		return
	}

	// Update indicators
	t.atr.Add(&indicators.Candle{
		Start: bar.Timestamp, Open: bar.Open, High: bar.High,
		Low: bar.Low, Close: bar.Close, Volume: bar.Volume,
	})
	t.momentum.Add(bar.Close)
	t.maxPrice.Add(bar.Timestamp, bar.High)

	t.checkOrders(bar)

	now := clocky.Now()
	openTime := cboe.GetOpenTime(now)
	closeTime := cboe.GetCloseTime(now)

	if now < openTime || now >= closeTime {
		return
	}

	// Close 10 mins before EOD
	if closeTime.Sub(now) <= 10*clocky.Minute {
		t.closePosition(bar, "EOD")
		return
	}

	if t.shares.IsPositive() {
		t.managePosition(bar)
	} else {
		t.lookForEntry(bar, now, openTime)
	}
}

func (t *Trader) lookForEntry(bar *ds.Bar, now clocky.Time, openTime clocky.Time) {
	if t.entryOrder != nil && t.entryOrder.Status.IsOpen() {
		return
	}

	sinceOpen := now.Sub(openTime)
	if sinceOpen < *flagEntryStart || sinceOpen >= *flagEntryEnd {
		return
	}

	if !t.atr.IsReady() || !t.momentum.IsReady() || !t.maxPrice.IsReady() {
		return
	}

	// Entry: new high AND positive momentum
	isNewHigh := bar.High.Cmp(t.maxPrice.Value) >= 0
	hasMomentum := t.momentum.Value.IsPositive()

	if !isNewHigh || !hasMomentum {
		return
	}

	// Size position
	portfolioValue := cubby.GetPortfolioValue()
	targetValue := portfolioValue.Mul(*flagPositionPct)
	limitPrice := bar.Close.QuantizeNearest(decimal.Cent)
	targetShares := targetValue.Div(limitPrice).Truncate()

	maxShares := t.equity.GetMaxOrderQuantity(limitPrice)
	if targetShares.Cmp(maxShares) > 0 {
		targetShares = maxShares
	}
	if !targetShares.IsPositive() {
		return
	}

	order, err := t.equity.Order(targetShares, limitPrice)
	if err != nil {
		return
	}
	t.entryOrder = order

	if cubby.Verbose {
		log.Printf("%s: ENTRY %s @ $%s (momentum=%s)",
			t.equity.Symbol, targetShares, limitPrice, t.momentum.Value)
	}
}

func (t *Trader) managePosition(bar *ds.Bar) {
	if t.exitOrder != nil && t.exitOrder.Status.IsOpen() {
		return
	}

	// Update trailing stop
	if bar.High.Cmp(t.peakPrice) > 0 {
		t.peakPrice = bar.High
		trail := t.peakPrice.Sub(t.atr.Value.Mul(*flagTrailATRMult))
		if trail.Cmp(t.stopPrice) > 0 {
			t.stopPrice = trail
		}
	}

	// Check stop
	if bar.Close.Cmp(t.stopPrice) <= 0 {
		t.closePosition(bar, "STOP")
	}
}

func (t *Trader) closePosition(bar *ds.Bar, reason string) {
	if t.exitOrder != nil && t.exitOrder.Status.IsOpen() {
		return
	}

	toClose := t.shares
	if !toClose.IsPositive() {
		toClose = t.equity.Quantity
	}
	if !toClose.IsPositive() {
		return
	}

	limitPrice := bar.Close.Mul(decimal.Parse("0.998")).QuantizeNearest(decimal.Cent)
	order, err := t.equity.Order(toClose.Neg(), limitPrice)
	if err != nil {
		return
	}
	t.exitOrder = order

	if cubby.Verbose && t.entry.IsPositive() {
		pnl := bar.Close.Sub(t.entry).Div(t.entry).MulInt(10000)
		log.Printf("%s: %s %s @ $%s (pnl %.0f bps)",
			t.equity.Symbol, reason, toClose, limitPrice, pnl.Float64())
	}
}

func (t *Trader) checkOrders(bar *ds.Bar) {
	if t.entryOrder != nil {
		if t.entryOrder.Status == alpaca.OrderStatusFilled {
			t.entry = t.entryOrder.FilledPrice
			t.shares = t.entryOrder.FilledQuantity
			t.peakPrice = bar.Close
			t.stopPrice = t.entry.Mul(decimal.One.Sub(*flagStopLoss))
			if cubby.Verbose {
				log.Printf("%s: FILLED %s @ $%s", t.equity.Symbol, t.shares, t.entry)
			}
			t.entryOrder = nil
		} else if t.entryOrder.Status.IsFinal() {
			t.entryOrder = nil
		} else if clocky.Now().Sub(t.entryOrder.OrderedAt) >= orderStaleness {
			t.entryOrder.Cancel()
			t.entryOrder = nil
		}
	}

	if t.exitOrder != nil {
		if t.exitOrder.Status == alpaca.OrderStatusFilled {
			if t.entry.IsPositive() && t.shares.IsPositive() {
				pnl := t.exitOrder.FilledPrice.Sub(t.entry).Mul(t.shares)
				t.dailyPnL = t.dailyPnL.Add(pnl)
			}
			t.shares = decimal.Zero
			t.entry = decimal.Zero
			t.peakPrice = decimal.Zero
			t.stopPrice = decimal.Zero
			t.exitOrder = nil
		} else if t.exitOrder.Status.IsFinal() {
			t.exitOrder = nil
		}
	}
}
