//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|             trading algorithm x3.102-2026

// holder is a momentum-weighted buy-and-hold strategy.
//
// Usage:
//
//	go run ./cmd/holder -backtest -start 2025-12-01 -symbol "GOOG AMZN GLD"
//
// It holds positions overnight, maximizing leverage during the day and
// reducing to overnight margin limits before close. Positions are weighted
// by momentum - winners get more capital, losers get less.
package main

import (
	"flag"
	"log"

	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"dropbear/indicators"
	"dropbear/loggy"
)

var (
	flagSymbols       = flag.String("symbol", "GOOG", "symbols to hold (space-separated)")
	flagMomentum      = flag.Int("momentum", 20, "momentum lookback period in bars")
	flagRebalance     = flag.Int("rebalance", 30, "minutes between rebalance checks")
	flagDayLeverage   = decimal.Flag("day-leverage", "2", "max intraday leverage")
	flagNightLeverage = decimal.Flag("night-leverage", "2", "overnight leverage (leave room for day trading)")
	flagMinWeight     = decimal.Flag("min-weight", "0.1", "minimum weight per symbol (0-1)")
	flagEqualWeight   = flag.Bool("equal-weight", false, "use equal weights instead of momentum-weighted")
)

const (
	marginCheckTime = 10 * clocky.Minute
	orderStaleness  = 5 * clocky.Minute
)

var (
	flagSlipClose = decimal.FlagBPS("slip-close", "50", "slippage on close orders")
	flagUrgency   = decimal.FlagBPS("urgency", "100", "extra slippage per minute when closing late")
)

// Opportunistic dip-buying flags
var (
	flagDipMin      = decimal.FlagBPS("dip-min", "200", "minimum dip from open to trigger (bps)")
	flagDipWindow   = clocky.DurationFlag("dip-window", "1h", "time window from open to look for dips")
	flagDipDeadline = clocky.DurationFlag("dip-deadline", "3h30m", "max time from open to keep dip order (10am PT = 3h30m from 6:30)")
	flagRedBars     = flag.Int("red-bars", 0, "consecutive red bars needed before entry")
	flagRecoveryMin = decimal.FlagBPS("recovery-min", "0", "min recovery as % of dip to confirm reversal (500 = 5%)")
	flagRecoveryMax = decimal.FlagBPS("recovery-max", "2000", "max recovery as % of dip to still enter (2000 = 20%)")
	flagTrailStop   = decimal.FlagBPS("trail-stop", "150", "drawdown from peak to exit dip trade (bps)")
	flagDipGreed    = decimal.FlagBPS("dip-greed", "10", "greed on dip entry (subtracted from current price)")
	flagExitGreed   = decimal.FlagBPS("exit-greed", "10", "greed on dip exit (subtracted from current price)")
)

var (
	gHoldings      []*Holding
	gDate          int
	gLastRebalance clocky.Time
	gTotalMomentum decimal.Decimal
)

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	if *flagSymbols == "" {
		log.Fatal("usage: holder -symbol \"GOOG AMZN GLD\" [-backtest] [-start DATE]")
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
		holding := NewHolding(equity)
		gHoldings = append(gHoldings, holding)
		equity.OnBar = holding.onBar
	}

	if len(gHoldings) == 0 {
		log.Fatal("no valid symbols to hold")
	}

	log.Printf("Momentum-Weighted Hold Strategy")
	log.Printf("  Symbols: %d", len(gHoldings))
	for _, h := range gHoldings {
		log.Printf("    %s", h.equity.Symbol)
	}
	log.Printf("  Day leverage: %s / Night leverage: %s", *flagDayLeverage, *flagNightLeverage)

	cubby.Run()
}

type Holding struct {
	equity        *cubby.Equity
	order         *cubby.Order
	monotonicTime clocky.Time
	momentum      *indicators.Momentum
	targetWeight  decimal.Decimal // 0-1, portion of portfolio
	barCount      int
	lastDate      int // track last date seen for per-holding reset

	// Dip-buying state
	openPrice     decimal.Decimal // first bar's open price for the day
	lowestPrice   decimal.Decimal // lowest close seen so far today
	redBarCount   int             // consecutive red bars
	sawRedBars    bool            // have we seen enough red bars?
	dipOrder      *cubby.Order    // opportunistic buy order (separate from rebalance order)
	dipEntryPrice decimal.Decimal // price we entered the dip trade
	dipShares     decimal.Decimal // shares from the opportunistic buy
	dipPeakPrice  decimal.Decimal // highest price since entry (for trailing stop)
}

func NewHolding(equity *cubby.Equity) *Holding {
	n := len(gHoldings) + 1 // +1 because we're being added
	return &Holding{
		equity:       equity,
		momentum:     indicators.NewMomentum(*flagMomentum),
		targetWeight: decimal.One.DivInt(n), // start equal
	}
}

func (h *Holding) onBar(bar *ds.Bar) {
	if !bar.Timestamp.After(h.monotonicTime) {
		return
	}
	h.monotonicTime = bar.Timestamp

	// new day reset (global close times + per-holding barCount)
	date := bar.Timestamp.DateInt()
	if date != gDate {
		gDate = date
	}
	if date != h.lastDate {
		h.lastDate = date
		h.barCount = 0
		// reset dip-buying state (actual prices set at market open)
		h.openPrice = decimal.Zero
		h.lowestPrice = decimal.Zero
		h.redBarCount = 0
		h.sawRedBars = false
		h.dipOrder = nil
		h.dipEntryPrice = decimal.Zero
		h.dipShares = decimal.Zero
		h.dipPeakPrice = decimal.Zero
	}

	if cubby.IsWarmingUp {
		return
	}

	// update momentum indicator
	h.momentum.Add(bar.Close)
	h.barCount++

	// check order status and handle stale orders
	if h.order != nil {
		if h.order.Status.IsFinal() {
			h.order = nil
		} else if clocky.Now().Sub(h.order.OrderedAt) >= orderStaleness {
			h.order.Cancel()
			h.order = nil
		}
	}

	now := clocky.Now()
	openTime := cubby.GetOpenTime(now)
	closeTime := cubby.GetCloseTime(now)
	if now < openTime || now >= closeTime {
		return
	}

	// before close: aggressively reduce to overnight leverage
	if closeTime.Sub(now) <= marginCheckTime {
		h.reduceForOvernight(now, closeTime)
		return
	}

	// opportunistic dip-buying
	h.handleDipTrading(bar, now, openTime)

	// during day: rebalance periodically to day leverage
	if now.Sub(gLastRebalance) >= clocky.Duration(*flagRebalance)*clocky.Minute {
		gLastRebalance = now
		updateWeights()
		// rebalance ALL holdings, not just the one that triggered
		for _, holding := range gHoldings {
			holding.rebalanceToLeverage(*flagDayLeverage)
		}
	}
}

// updateWeights calculates momentum-weighted target allocations
func updateWeights() {
	n := len(gHoldings)
	equalWeight := decimal.One.DivInt(n)

	// equal weight mode: skip momentum calculations entirely
	if *flagEqualWeight {
		for _, h := range gHoldings {
			h.targetWeight = equalWeight
		}
		if cubby.Verbose {
			log.Printf("updated weights: equal %.2f%% each", equalWeight.MulInt(100).Float64())
		}
		return
	}

	// calculate total positive momentum
	gTotalMomentum = decimal.Zero
	for _, h := range gHoldings {
		if h.momentum.IsReady() && h.momentum.Value.IsPositive() {
			gTotalMomentum = gTotalMomentum.Add(h.momentum.Value)
		}
	}

	// clamp minWeight so that minWeight * n <= 0.5 (leave room for momentum)
	minWeight := (*flagMinWeight).Min(decimal.Half.DivInt(n))
	remainingWeight := decimal.One.Sub(minWeight.MulInt(n))

	for _, h := range gHoldings {
		if !h.momentum.IsReady() || gTotalMomentum.IsZero() {
			// equal weight if no momentum data
			h.targetWeight = equalWeight
		} else if h.momentum.Value.IsPositive() {
			// momentum-weighted: min weight + share of remaining based on momentum
			momentumShare := h.momentum.Value.Div(gTotalMomentum)
			h.targetWeight = minWeight.Add(remainingWeight.Mul(momentumShare))
		} else {
			// negative momentum: just minimum weight
			h.targetWeight = minWeight
		}
	}

	// always normalize weights to sum to 1
	totalWeight := decimal.Zero
	for _, h := range gHoldings {
		totalWeight = totalWeight.Add(h.targetWeight)
	}
	if !totalWeight.IsPositive() {
		// fallback to equal weights if something went wrong
		for _, h := range gHoldings {
			h.targetWeight = equalWeight
		}
	} else {
		for _, h := range gHoldings {
			h.targetWeight = h.targetWeight.Div(totalWeight)
		}
	}

	if cubby.Verbose {
		log.Printf("updated weights:")
		for _, h := range gHoldings {
			mom := "n/a"
			if h.momentum.IsReady() {
				mom = h.momentum.Value.String()
			}
			log.Printf("  %s: weight=%.2f%% momentum=%s",
				h.equity.Symbol, h.targetWeight.MulInt(100).Float64(), mom)
		}
	}
}

func (h *Holding) rebalanceToLeverage(targetLeverage decimal.Decimal) {
	if h.order != nil && h.order.Status.IsOpen() {
		return // already have pending order
	}

	if !h.equity.Price.IsPositive() {
		return
	}

	// calculate target position value
	portfolioValue := cubby.GetPortfolioValue()
	targetExposure := portfolioValue.Mul(targetLeverage)
	targetValue := targetExposure.Mul(h.targetWeight)
	targetShares := targetValue.Div(h.equity.Price).Truncate()

	// calculate current position value
	currentShares := h.equity.Quantity
	diff := targetShares.Sub(currentShares)

	// only rebalance if difference is significant (>5% of target)
	if targetShares.IsPositive() {
		diffPct := diff.Abs().Div(targetShares)
		if diffPct.Cmp(decimal.Parse("0.05")) < 0 {
			return // close enough
		}
	} else if diff.IsZero() {
		return
	}

	// place order at midpoint
	limitPrice := h.equity.Price.QuantizeNearest(decimal.Cent)

	order, err := h.equity.Order(diff, limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("rebalance %s: error ordering %s shares: %v",
				h.equity.Symbol, diff, err)
		}
		return
	}
	h.order = order

	if cubby.Verbose {
		action := "buy"
		if diff.IsNegative() {
			action = "sell"
		}
		log.Printf("rebalance %s: %s %s shares (target weight %.1f%%)",
			h.equity.Symbol, action, diff.Abs(), h.targetWeight.MulInt(100).Float64())
	}
}

// reduceForOvernight aggressively reduces position to overnight margin requirements.
// Unlike rebalanceToLeverage, this function:
// 1. Cancels and replaces stale orders if price has moved
// 2. Uses urgency-based slippage that increases as we approach close
// 3. Doesn't use any threshold - acts on any difference
func (h *Holding) reduceForOvernight(now, closeTime clocky.Time) {
	if !h.equity.Price.IsPositive() {
		return
	}

	// calculate target position for overnight leverage
	portfolioValue := cubby.GetPortfolioValue()
	targetExposure := portfolioValue.Mul(*flagNightLeverage)
	targetValue := targetExposure.Mul(h.targetWeight)
	targetShares := targetValue.Div(h.equity.Price).Truncate()
	currentShares := h.equity.Quantity
	diff := targetShares.Sub(currentShares)

	// nothing to do if we're at or below target
	if !diff.IsNegative() {
		return
	}

	// calculate urgency: 0 at marginCheckTime, 1 at closeTime
	urgency := decimal.Zero
	timeLeft := closeTime.Sub(now)
	if timeLeft > 0 {
		timeRemaining := decimal.FromInt64(timeLeft.Milliseconds())
		totalWindow := decimal.FromInt64(marginCheckTime.Milliseconds())
		urgency = timeRemaining.Div(totalWindow)
	}
	adaptiveSlip := flagSlipClose.Add(urgency.Mul(*flagUrgency))

	// check if we should cancel and replace stale order
	if h.order != nil && h.order.Status.IsOpen() {
		if h.order.Side == ds.SideSell {
			// cancel if price moved significantly
			priceDiff := h.equity.Price.Sub(h.order.LimitPrice).Div(h.equity.Price).Abs()
			if priceDiff.Cmp(decimal.Parse("0.005")) > 0 {
				h.order.Cancel()
				h.order = nil
			} else {
				return // order is still good
			}
		} else {
			// we have a buy order but need to sell - let it be
			return
		}
	}

	// place sell order with urgency-based slippage
	limitPrice := h.equity.Price.Mul(decimal.One.Sub(adaptiveSlip))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	sellQty := diff.Abs()                          // diff is negative, so abs gives us shares to sell
	order, err := h.equity.Order(diff, limitPrice) // diff is negative = sell
	if err != nil {
		if cubby.Verbose {
			log.Printf("EOD reduce %s: error selling %s shares: %v",
				h.equity.Symbol, sellQty, err)
		}
		return
	}
	h.order = order

	if cubby.Verbose {
		log.Printf("EOD reduce %s: sell %s shares at $%s (urgency %.0f%%, slip %.2f%%)",
			h.equity.Symbol, sellQty, limitPrice,
			urgency.MulInt(100).Float64(), adaptiveSlip.MulInt(100).Float64())
	}
}

// handleDipTrading implements opportunistic dip-buying strategy.
// 1. Look for dip from open (configurable min bps)
// 2. Wait for N consecutive red bars
// 3. Enter on first green bar if recovery is limited
// 4. Use trailing stop to exit
func (h *Holding) handleDipTrading(bar *ds.Bar, now clocky.Time, openTime clocky.Time) {
	// initialize open/lowest prices at market open
	if h.openPrice.IsZero() {
		h.openPrice = bar.Open
		h.lowestPrice = bar.Low
	}

	// check if we're in an active dip trade (have shares to manage)
	if h.dipShares.IsPositive() {
		h.manageDipExit(bar)
		return
	}

	// check if dip order filled
	if h.dipOrder != nil {
		if h.dipOrder.Status == alpaca.OrderStatusFilled {
			h.dipEntryPrice = h.dipOrder.FilledPrice
			h.dipShares = h.dipOrder.FilledQuantity
			h.dipPeakPrice = bar.Close
			h.dipOrder = nil
			if cubby.Verbose {
				log.Printf("dip %s: filled %s shares at $%s",
					h.equity.Symbol, h.dipShares, h.dipEntryPrice)
			}
			return
		} else if h.dipOrder.Status.IsFinal() {
			// order canceled or expired
			h.dipOrder = nil
		}
	}

	// past deadline? cancel any pending dip order
	if now.Sub(openTime) >= *flagDipDeadline {
		if h.dipOrder != nil && h.dipOrder.Status.IsOpen() {
			h.dipOrder.Cancel()
			h.dipOrder = nil
		}
		return
	}

	// already have a pending dip order? let it ride
	if h.dipOrder != nil && h.dipOrder.Status.IsOpen() {
		return
	}

	// only look for dips within the dip window
	if now.Sub(openTime) > *flagDipWindow {
		return
	}

	// use defer to always update lowest price before returning
	defer func() {
		if bar.Low.Cmp(h.lowestPrice) < 0 {
			h.lowestPrice = bar.Low
		}
	}()

	// track consecutive red bars (for optional filtering)
	isRedBar := bar.Close.Cmp(bar.Open) < 0
	if isRedBar {
		h.redBarCount++
	} else {
		if h.redBarCount >= *flagRedBars {
			h.sawRedBars = true
		}
		h.redBarCount = 0
	}

	// check entry conditions BEFORE updating lowest (so we measure recovery from prior low)
	// only check if we're not making new lows (bar.Low >= lowestPrice means dip may be ending)
	if h.openPrice.IsPositive() && h.sawRedBars && h.lowestPrice.IsPositive() && bar.Low.Cmp(h.lowestPrice) >= 0 {
		// calculate dip from open
		dipBps := h.openPrice.Sub(h.lowestPrice).Div(h.openPrice).MulInt(10000)
		dipMinBps := flagDipMin.MulInt(10000)

		// is the dip big enough?
		if dipBps.Cmp(dipMinBps) < 0 {
			return // dip not deep enough
		}

		// calculate recovery from lowest
		recovery := bar.Close.Sub(h.lowestPrice)
		dipSize := h.openPrice.Sub(h.lowestPrice)
		if !dipSize.IsPositive() {
			return
		}
		recoveryPct := recovery.Div(dipSize).MulInt(10000) // as bps (100% = 10000)

		// is recovery in the sweet spot? (enough to confirm reversal, not too much to miss it)
		// flagRecoveryMin/Max are in decimal form (0.05 = 5%), recoveryPct is in bps (500 = 5%)
		recoveryMinBps := flagRecoveryMin.MulInt(10000)
		recoveryMaxBps := flagRecoveryMax.MulInt(10000)
		if recoveryPct.Cmp(recoveryMinBps) < 0 {
			return // not enough recovery yet, reversal not confirmed
		}
		if recoveryPct.Cmp(recoveryMaxBps) > 0 {
			return // recovered too much, missed the opportunity
		}

		// calculate greed based on dip size
		// if dipped 50 bps and recovered 10 bps, greed could be -10 bps
		greed := *flagDipGreed
		limitPrice := bar.Close.Mul(decimal.One.Sub(greed))
		limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

		// calculate max shares we can buy with available DTBP
		maxShares := h.equity.GetMaxOrderQuantity(limitPrice)
		if !maxShares.IsPositive() {
			return // no buying power available
		}

		// place the order
		order, err := h.equity.Order(maxShares, limitPrice)
		if err != nil {
			if cubby.Verbose {
				log.Printf("dip %s: error ordering %s shares at $%s: %v",
					h.equity.Symbol, maxShares, limitPrice, err)
			}
			return
		}
		h.dipOrder = order

		if cubby.Verbose {
			log.Printf("dip %s: buying %s shares at $%s (dip %.0f bps, recovery %.0f%%)",
				h.equity.Symbol, maxShares, limitPrice,
				dipBps.Float64(), recoveryPct.DivInt(100).Float64())
		}
	}
}

// manageDipExit implements trailing stop for dip trades
func (h *Holding) manageDipExit(bar *ds.Bar) {
	// update peak price
	if bar.Close.Cmp(h.dipPeakPrice) > 0 {
		h.dipPeakPrice = bar.Close
	}

	// calculate drawdown from peak
	if !h.dipPeakPrice.IsPositive() {
		return
	}
	drawdownBps := h.dipPeakPrice.Sub(bar.Close).Div(h.dipPeakPrice).MulInt(10000)

	// check if we should exit
	if drawdownBps.Cmp(flagTrailStop.MulInt(10000)) < 0 {
		return // haven't hit stop yet
	}

	// place exit order with greed
	limitPrice := bar.Close.Mul(decimal.One.Sub(*flagExitGreed))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	order, err := h.equity.Order(h.dipShares.Neg(), limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("dip exit %s: error selling %s shares: %v",
				h.equity.Symbol, h.dipShares, err)
		}
		return
	}

	profit := h.dipPeakPrice.Sub(h.dipEntryPrice).Div(h.dipEntryPrice).MulInt(10000)
	if cubby.Verbose {
		log.Printf("dip exit %s: selling %s shares at $%s (peak $%s, profit %.0f bps)",
			h.equity.Symbol, h.dipShares, limitPrice, h.dipPeakPrice, profit.Float64())
	}

	// reset dip state - shares will be removed when order fills
	// but we track via the order for now
	h.dipShares = decimal.Zero
	h.dipEntryPrice = decimal.Zero
	h.dipPeakPrice = decimal.Zero
	h.order = order // use the regular order slot for the exit
}
