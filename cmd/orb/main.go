//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|             trading algorithm x3.104-2026

// orb implements Opening Range Breakout strategy.
//
// Usage:
//
//	go run ./cmd/orb -backtest -start 2025-10-01 -symbol "GOOG"
//
// Strategy:
//   - First N minutes establish the opening range (high/low)
//   - Buy when price breaks above the range high
//   - Trail stop based on ATR
//   - Close all positions before market close
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
	flagSymbols = flag.String("symbol", "GOOG", "symbols to trade (space-separated)")

	// Opening range parameters
	flagRangeMins     = flag.Int("range-mins", 15, "minutes to establish opening range")
	flagBreakoutPct   = decimal.FlagBPS("breakout", "10", "buffer above range high for entry (bps)")
	flagEntryDeadline = clocky.DurationFlag("entry-deadline", "3h", "latest entry after open (9:30am PT)")

	// Exit parameters
	flagTrailATRMult = decimal.Flag("trail-atr-mult", "2", "ATR multiplier for trail stop")
	flagATRPeriod    = flag.Int("atr-period", 14, "ATR calculation period")
	flagProfitTarget = decimal.FlagBPS("profit-target", "0", "take profit at this gain (0=disabled)")
	flagStopLoss     = decimal.FlagBPS("stop-loss", "100", "hard stop loss below entry (bps)")

	// Risk parameters
	flagPositionPct  = decimal.Flag("position-pct", "0.5", "position size as fraction of portfolio")
	flagMaxDailyLoss = decimal.FlagBPS("max-daily-loss", "200", "stop trading after this loss (bps)")
	flagCooldown     = clocky.DurationFlag("cooldown", "30m", "wait after loss before re-entry")
)

const orderStaleness = 2 * clocky.Minute

var (
	gTraders []*Trader
	gDate    int
)

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	if *flagSymbols == "" {
		log.Fatal("usage: orb -symbol \"GOOG\" [-backtest] [-start DATE]")
	}

	syms, err := symbol.Expand(*flagSymbols)
	if err != nil {
		log.Fatalf("invalid symbols: %v", err)
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
		log.Fatal("no valid symbols")
	}

	log.Printf("Opening Range Breakout Strategy")
	log.Printf("  Symbols: %d", len(gTraders))
	log.Printf("  Range: %d mins, Trail: %sx ATR", *flagRangeMins, *flagTrailATRMult)

	cubby.Run()
}

type Trader struct {
	equity   *cubby.Equity
	lastTime clocky.Time
	lastDate int

	// Indicators
	atr *indicators.ATR

	// Opening range state (reset daily)
	rangeHigh   decimal.Decimal
	rangeLow    decimal.Decimal
	rangeSet    bool
	barsInRange int

	// Position state
	entryOrder *cubby.Order
	exitOrder  *cubby.Order
	entry      decimal.Decimal
	shares     decimal.Decimal
	peakPrice  decimal.Decimal
	stopPrice  decimal.Decimal // hard stop

	// Daily state
	dailyPnL    decimal.Decimal
	lastLossAt  clocky.Time
	tradedToday bool
}

func NewTrader(equity *cubby.Equity) *Trader {
	return &Trader{
		equity: equity,
		atr:    indicators.NewATR(*flagATRPeriod),
	}
}

func (t *Trader) onBar(bar *ds.Bar) {
	if !bar.Timestamp.After(t.lastTime) {
		return
	}
	t.lastTime = bar.Timestamp

	// New day reset
	date := bar.Timestamp.DateInt()
	if date != gDate {
		gDate = date
	}
	if date != t.lastDate {
		t.lastDate = date
		t.rangeHigh = decimal.Zero
		t.rangeLow = decimal.Zero
		t.rangeSet = false
		t.barsInRange = 0
		t.dailyPnL = decimal.Zero
		t.tradedToday = false
	}

	if cubby.IsWarmingUp {
		return
	}

	// Update ATR
	t.atr.Add(&indicators.Candle{
		Start:  bar.Timestamp,
		Open:   bar.Open,
		High:   bar.High,
		Low:    bar.Low,
		Close:  bar.Close,
		Volume: bar.Volume,
	})

	t.checkOrders(bar)

	now := clocky.Now()
	openTime := cboe.GetOpenTime(now)
	closeTime := cboe.GetCloseTime(now)

	if now < openTime || now >= closeTime {
		return
	}

	// Close positions 10 mins before close
	if closeTime.Sub(now) <= 10*clocky.Minute {
		t.closePosition(bar, "EOD")
		return
	}

	// Build opening range
	if !t.rangeSet {
		t.buildRange(bar, now, openTime)
		return
	}

	// Manage position or look for entry
	if t.shares.IsPositive() {
		t.managePosition(bar, now)
	} else {
		t.lookForEntry(bar, now, openTime)
	}
}

func (t *Trader) buildRange(bar *ds.Bar, now clocky.Time, openTime clocky.Time) {
	sinceOpen := now.Sub(openTime)

	// Update range high/low
	if t.rangeHigh.IsZero() || bar.High.Cmp(t.rangeHigh) > 0 {
		t.rangeHigh = bar.High
	}
	if t.rangeLow.IsZero() || bar.Low.Cmp(t.rangeLow) < 0 {
		t.rangeLow = bar.Low
	}
	t.barsInRange++

	// Check if range period complete
	rangeDuration := clocky.Duration(*flagRangeMins) * clocky.Minute
	if sinceOpen >= rangeDuration {
		t.rangeSet = true
		if cubby.Verbose {
			rangeSize := t.rangeHigh.Sub(t.rangeLow).Div(t.rangeLow).MulInt(10000)
			log.Printf("%s: range set H=$%s L=$%s (%.0f bps)",
				t.equity.Symbol, t.rangeHigh, t.rangeLow, rangeSize.Float64())
		}
	}
}

func (t *Trader) lookForEntry(bar *ds.Bar, now clocky.Time, openTime clocky.Time) {
	// Already have pending order
	if t.entryOrder != nil && t.entryOrder.Status.IsOpen() {
		return
	}

	// Past entry deadline
	if now.Sub(openTime) >= *flagEntryDeadline {
		return
	}

	// Already traded today (one trade per day keeps it simple)
	if t.tradedToday {
		return
	}

	// Check cooldown after loss
	if !t.lastLossAt.IsZero() && now.Sub(t.lastLossAt) < *flagCooldown {
		return
	}

	// Check daily loss limit
	maxLoss := cubby.GetPortfolioValue().Mul(*flagMaxDailyLoss)
	if t.dailyPnL.Neg().Cmp(maxLoss) >= 0 {
		return
	}

	// Need ATR for stop calculation
	if !t.atr.IsReady() {
		return
	}

	// Check for breakout above range high
	breakoutLevel := t.rangeHigh.Mul(decimal.One.Add(*flagBreakoutPct))
	if bar.Close.Cmp(breakoutLevel) <= 0 {
		return // No breakout yet
	}

	// Calculate position size
	portfolioValue := cubby.GetPortfolioValue()
	targetValue := portfolioValue.Mul(*flagPositionPct)
	limitPrice := bar.Close.Add(t.atr.Value.MulInt(1).DivInt(10)) // small buffer
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)
	targetShares := targetValue.Div(limitPrice).Truncate()

	maxShares := t.equity.GetMaxOrderQuantity(limitPrice)
	if targetShares.Cmp(maxShares) > 0 {
		targetShares = maxShares
	}
	if !targetShares.IsPositive() {
		return
	}

	// Place entry order
	order, err := t.equity.Order(targetShares, limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("%s: entry error: %v", t.equity.Symbol, err)
		}
		return
	}
	t.entryOrder = order
	t.tradedToday = true

	if cubby.Verbose {
		breakoutPct := bar.Close.Sub(t.rangeHigh).Div(t.rangeHigh).MulInt(10000)
		log.Printf("%s: BREAKOUT %s shares @ $%s (%.0f bps above range)",
			t.equity.Symbol, targetShares, limitPrice, breakoutPct.Float64())
	}
}

func (t *Trader) managePosition(bar *ds.Bar, now clocky.Time) {
	if t.exitOrder != nil && t.exitOrder.Status.IsOpen() {
		return
	}

	// Update peak and trailing stop
	if bar.High.Cmp(t.peakPrice) > 0 {
		t.peakPrice = bar.High
		// Trail the stop up
		trailStop := t.peakPrice.Sub(t.atr.Value.Mul(*flagTrailATRMult))
		if trailStop.Cmp(t.stopPrice) > 0 {
			t.stopPrice = trailStop
		}
	}

	// Check profit target
	if flagProfitTarget.IsPositive() {
		targetPrice := t.entry.Mul(decimal.One.Add(*flagProfitTarget))
		if bar.Close.Cmp(targetPrice) >= 0 {
			t.closePosition(bar, "TARGET")
			return
		}
	}

	// Check stop loss
	if bar.Close.Cmp(t.stopPrice) <= 0 {
		t.closePosition(bar, "STOP")
		t.lastLossAt = now
		return
	}
}

func (t *Trader) closePosition(bar *ds.Bar, reason string) {
	if t.exitOrder != nil && t.exitOrder.Status.IsOpen() {
		return
	}

	sharesToClose := t.shares
	if !sharesToClose.IsPositive() {
		sharesToClose = t.equity.Quantity
	}
	if !sharesToClose.IsPositive() {
		return
	}

	// Aggressive limit for quick fill
	limitPrice := bar.Close.Mul(decimal.One.Sub(decimal.Parse("0.002")))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	order, err := t.equity.Order(sharesToClose.Neg(), limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("%s: exit error: %v", t.equity.Symbol, err)
		}
		return
	}
	t.exitOrder = order

	if cubby.Verbose && t.entry.IsPositive() {
		pnl := bar.Close.Sub(t.entry).Div(t.entry).MulInt(10000)
		log.Printf("%s: %s exit %s @ $%s (pnl %.0f bps)",
			t.equity.Symbol, reason, sharesToClose, limitPrice, pnl.Float64())
	}
}

func (t *Trader) checkOrders(bar *ds.Bar) {
	// Check entry order
	if t.entryOrder != nil {
		if t.entryOrder.Status == alpaca.OrderStatusFilled {
			t.entry = t.entryOrder.FilledPrice
			t.shares = t.entryOrder.FilledQuantity
			t.peakPrice = bar.Close
			// Set initial stop below entry
			t.stopPrice = t.entry.Mul(decimal.One.Sub(*flagStopLoss))
			if cubby.Verbose {
				log.Printf("%s: FILLED entry %s @ $%s (stop $%s)",
					t.equity.Symbol, t.shares, t.entry, t.stopPrice)
			}
			t.entryOrder = nil
		} else if t.entryOrder.Status.IsFinal() {
			t.entryOrder = nil
		} else if clocky.Now().Sub(t.entryOrder.OrderedAt) >= orderStaleness {
			t.entryOrder.Cancel()
			t.entryOrder = nil
			t.tradedToday = false // Allow retry
		}
	}

	// Check exit order
	if t.exitOrder != nil {
		if t.exitOrder.Status == alpaca.OrderStatusFilled {
			exitPrice := t.exitOrder.FilledPrice
			if t.entry.IsPositive() && t.shares.IsPositive() {
				pnlDollars := exitPrice.Sub(t.entry).Mul(t.shares)
				t.dailyPnL = t.dailyPnL.Add(pnlDollars)
				if cubby.Verbose {
					pnlBps := exitPrice.Sub(t.entry).Div(t.entry).MulInt(10000)
					log.Printf("%s: FILLED exit @ $%s (pnl %.0f bps, day $%.2f)",
						t.equity.Symbol, exitPrice, pnlBps.Float64(), t.dailyPnL.Float64())
				}
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
