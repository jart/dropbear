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
	"fmt"
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
	flagSymbols       = flag.String("symbol", "GOOG", "symbols to hold (space-separated)")
	flagMomentum      = flag.Int("momentum", 20, "momentum lookback period in bars")
	flagRebalance     = flag.Int("rebalance", 30, "minutes between rebalance checks")
	flagNightLeverage = decimal.Flag("night-leverage", "2", "overnight leverage (leave room for day trading)")
	flagMinWeight     = decimal.Flag("min-weight", "0.1", "minimum weight per symbol (0-1)")
	flagEqualWeight   = flag.Bool("equal-weight", false, "use equal weights instead of momentum-weighted")
)

const (
	orderStaleness = 5 * clocky.Minute
)

var (
	// End-of-day position management
	flagNoBuyTime = clocky.DurationFlag("no-buy-time", "30m", "stop buying this long before close")
	flagLOCTime   = clocky.DurationFlag("loc-time", "11m", "time before close to place limit-on-close orders")
	flagLOCSlip   = decimal.FlagBPS("loc-slip", "300", "slippage for limit-on-close orders (bps)")
)

// Opportunistic intraday trading flags
var (
	flagDipEnabled  = flag.Bool("dip", false, "enable opportunistic dip-buying (mean reversion)")
	flagMomoEnabled = flag.Bool("momo", false, "enable momentum buying (trend following)")
	flagDipMin      = decimal.FlagBPS("dip-min", "150", "minimum dip from open to trigger (bps)")
	flagMomoMin     = decimal.FlagBPS("momo-min", "100", "minimum gain from open to trigger momentum buy (bps)")
	flagDipWindow   = clocky.DurationFlag("dip-window", "1h", "time window from open to look for dips")
	flagMomoWindow  = clocky.DurationFlag("momo-window", "2h", "time window from open to look for momentum")
	flagDipDeadline = clocky.DurationFlag("dip-deadline", "3h30m", "max time from open to keep position (10am PT)")
	flagRedBars     = flag.Int("red-bars", 0, "consecutive red bars needed before dip entry")
	flagGreenBars   = flag.Int("green-bars", 2, "consecutive green bars needed before momentum entry")
	flagRecoveryMin = decimal.FlagBPS("recovery-min", "0", "min recovery as % of dip to confirm reversal (500 = 5%)")
	flagRecoveryMax = decimal.FlagBPS("recovery-max", "2000", "max recovery as % of dip to still enter (2000 = 20%)")
	flagTrailStop   = decimal.FlagBPS("trail-stop", "150", "drawdown from peak to exit trade (bps)")
	flagDipMaxPct   = decimal.Flag("dip-max-pct", "0.5", "max intraday position as fraction of portfolio (0.5 = 50%)")
	flagDipGreed    = decimal.FlagBPS("dip-greed", "10", "greed on entry (subtracted from current price)")
	flagExitGreed   = decimal.FlagBPS("exit-greed", "10", "greed on exit (subtracted from current price)")
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
	log.Printf("  Night leverage: %s", *flagNightLeverage)

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

	// Intraday trading state
	openPrice      decimal.Decimal // first bar's open price for the day
	lowestPrice    decimal.Decimal // lowest close seen so far today
	highestPrice   decimal.Decimal // highest close seen so far today
	redBarCount    int             // consecutive red bars
	greenBarCount  int             // consecutive green bars
	sawRedBars     bool            // have we seen enough red bars?
	sawGreenBars   bool            // have we seen enough green bars?
	dipOrder       *cubby.Order    // opportunistic buy order (separate from rebalance order)
	dipEntryPrice  decimal.Decimal // price we entered the intraday trade
	dipShares      decimal.Decimal // shares from the opportunistic buy
	dipPeakPrice   decimal.Decimal // highest price since entry (for trailing stop)
	intradayTraded bool            // already traded today (limit to one trade)
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
		// reset intraday trading state (actual prices set at market open)
		h.openPrice = decimal.Zero
		h.lowestPrice = decimal.Zero
		h.highestPrice = decimal.Zero
		h.redBarCount = 0
		h.greenBarCount = 0
		h.sawRedBars = false
		h.sawGreenBars = false
		h.dipOrder = nil
		h.dipEntryPrice = decimal.Zero
		h.dipShares = decimal.Zero
		h.dipPeakPrice = decimal.Zero
		h.intradayTraded = false
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
	year, month, day := now.Date()
	openTime := cboe.GetOpenTime(year, month, day)
	closeTime := cboe.GetCloseTime(year, month, day)
	if now < openTime || now >= closeTime {
		return
	}

	// before close: place limit-on-close orders to reduce to overnight leverage
	if closeTime.Sub(now) <= *flagLOCTime {
		h.reduceForOvernight(bar)
		return
	}

	// opportunistic intraday trading (if enabled)
	if *flagDipEnabled {
		h.handleDipTrading(bar, now, openTime)
	}
	if *flagMomoEnabled {
		h.handleMomoTrading(bar, now, openTime)
	}

	// during day: rebalance periodically to day leverage
	if now.Sub(gLastRebalance) >= clocky.Duration(*flagRebalance)*clocky.Minute {
		gLastRebalance = now
		updateWeights()
		logLeverageStatus()
		rebalanceAll()
	}
}

// rebalanceAll rebalances all holdings, allocating available BP proportionally by weight
func rebalanceAll() {
	now := clocky.Now()
	year, month, day := now.Date()
	closeTime := cboe.GetCloseTime(year, month, day)
	inNoBuyWindow := closeTime.Sub(now) <= *flagNoBuyTime

	for _, h := range gHoldings {
		if h.order != nil && h.order.Status.IsOpen() {
			continue
		}
		if !h.equity.Price.IsPositive() {
			continue
		}

		coreShares := h.equity.Quantity.Sub(h.dipShares)
		if coreShares.IsNegative() {
			coreShares = decimal.Zero
		}

		limitPrice := h.equity.Price.QuantizeNearest(decimal.Cent)

		// Use GetMaxOrderQuantity which properly handles per-symbol margin rates
		maxShares := h.equity.GetMaxOrderQuantity(limitPrice)
		// Scale by this holding's weight
		targetBuyShares := maxShares.Mul(h.targetWeight).Truncate()

		var diff decimal.Decimal
		if !inNoBuyWindow && targetBuyShares.IsPositive() {
			diff = targetBuyShares
		}
		// TODO: handle sells for rebalancing if needed

		if diff.IsZero() {
			continue
		}

		// Skip small changes
		if coreShares.IsPositive() {
			diffPct := diff.Abs().Div(coreShares)
			if diffPct.Cmp(decimal.Parse("0.05")) < 0 {
				continue
			}
		}

		order, err := h.equity.Order(diff, limitPrice)
		if err != nil {
			continue
		}
		h.order = order

		log.Printf("rebalance %s: buy %s shares @ $%s (have %s, weight %.1f%%)",
			h.equity.Symbol, diff, limitPrice, coreShares, h.targetWeight.MulInt(100).Float64())
	}
}

// logLeverageStatus logs current leverage and margin usage
func logLeverageStatus() {
	portfolioValue := cubby.GetPortfolioValue()
	investedValue := cubby.GetInvestedValue()
	marginUsed := cubby.GetMarginUsed()
	availableBP := cubby.GetAvailableBuyingPower()

	currentLeverage := decimal.Zero
	if portfolioValue.IsPositive() {
		currentLeverage = investedValue.Div(portfolioValue)
	}

	log.Printf("leverage: %.2fx | invested $%s | margin used $%s | available BP $%s",
		currentLeverage.Float64(),
		investedValue.FormatThousand(0),
		marginUsed.FormatThousand(0),
		availableBP.FormatThousand(0))
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

// reduceForOvernight places limit-on-close orders to reduce to overnight margin.
// Places a single aggressive order with loc-slip to ensure execution before close.
func (h *Holding) reduceForOvernight(bar *ds.Bar) {
	if !h.equity.Price.IsPositive() {
		return
	}

	// already have a pending sell order for EOD - let it ride
	if h.order != nil && h.order.Status.IsOpen() && h.order.Side == ds.SideSell {
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

	// prevent going short - don't sell more than we own
	if diff.Abs().Cmp(currentShares) > 0 {
		diff = currentShares.Neg()
		if diff.IsZero() {
			return
		}
	}

	// place aggressive limit-on-close order (crosses spread by loc-slip %)
	limitPrice := bar.Close.Mul(decimal.One.Sub(*flagLOCSlip))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	sellQty := diff.Abs()
	order, err := h.equity.Order(diff, limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("LOC %s: error selling %s shares: %v",
				h.equity.Symbol, sellQty, err)
		}
		return
	}
	h.order = order

	if cubby.Verbose {
		log.Printf("LOC %s: sell %s shares at $%s (%.0f bps below close)",
			h.equity.Symbol, sellQty, limitPrice, flagLOCSlip.MulInt(10000).Float64())
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

	// past deadline? cancel pending orders and close any open dip position
	if now.Sub(openTime) >= *flagDipDeadline {
		if h.dipOrder != nil && h.dipOrder.Status.IsOpen() {
			h.dipOrder.Cancel()
			h.dipOrder = nil
		}
		// force close any open dip position at deadline (but don't go short)
		if h.dipShares.IsPositive() {
			sellShares := h.dipShares
			if sellShares.Cmp(h.equity.Quantity) > 0 {
				sellShares = h.equity.Quantity
			}
			if !sellShares.IsPositive() {
				h.dipShares = decimal.Zero
				h.dipEntryPrice = decimal.Zero
				h.dipPeakPrice = decimal.Zero
				return
			}
			limitPrice := bar.Close.Mul(decimal.One.Sub(*flagExitGreed))
			limitPrice = limitPrice.QuantizeNearest(decimal.Cent)
			order, err := h.equity.Order(sellShares.Neg(), limitPrice)
			if err == nil {
				if cubby.Verbose {
					pnl := bar.Close.Sub(h.dipEntryPrice).Div(h.dipEntryPrice).MulInt(10000)
					log.Printf("dip deadline %s: closing %s shares at $%s (pnl %.0f bps)",
						h.equity.Symbol, h.dipShares, limitPrice, pnl.Float64())
				}
				h.order = order
			}
			h.dipShares = decimal.Zero
			h.dipEntryPrice = decimal.Zero
			h.dipPeakPrice = decimal.Zero
		}
		return
	}

	// check if we're in an active dip trade (have shares to manage)
	if h.dipShares.IsPositive() {
		h.manageDipExit(bar)
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

		// cap dip position to dip-max-pct of portfolio value
		portfolioValue := cubby.GetPortfolioValue()
		maxDipValue := portfolioValue.Mul(*flagDipMaxPct)
		maxDipShares := maxDipValue.Div(limitPrice).Truncate()
		if maxShares.Cmp(maxDipShares) > 0 {
			maxShares = maxDipShares
		}
		if !maxShares.IsPositive() {
			return
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

	// don't sell more than we actually have (prevent going short)
	sellShares := h.dipShares
	if sellShares.Cmp(h.equity.Quantity) > 0 {
		sellShares = h.equity.Quantity
	}
	if !sellShares.IsPositive() {
		h.dipShares = decimal.Zero
		h.dipEntryPrice = decimal.Zero
		h.dipPeakPrice = decimal.Zero
		return
	}

	// place exit order with greed
	limitPrice := bar.Close.Mul(decimal.One.Sub(*flagExitGreed))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	order, err := h.equity.Order(sellShares.Neg(), limitPrice)
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

// handleMomoTrading implements momentum-based intraday trading.
// Instead of buying dips, buy when the stock is UP from open (trend following).
func (h *Holding) handleMomoTrading(bar *ds.Bar, now clocky.Time, openTime clocky.Time) {
	// initialize prices at market open
	if h.openPrice.IsZero() {
		h.openPrice = bar.Open
		h.highestPrice = bar.High
	}

	// check if order filled
	if h.dipOrder != nil {
		if h.dipOrder.Status == alpaca.OrderStatusFilled {
			h.dipEntryPrice = h.dipOrder.FilledPrice
			h.dipShares = h.dipOrder.FilledQuantity
			h.dipPeakPrice = bar.Close
			h.dipOrder = nil
			log.Printf("momo %s: filled %s shares at $%s", h.equity.Symbol, h.dipShares, h.dipEntryPrice)
			logLeverageStatus()
			return
		} else if h.dipOrder.Status.IsFinal() {
			log.Printf("momo %s: order %s", h.equity.Symbol, h.dipOrder.Status)
			h.dipOrder = nil
		}
	}

	// past deadline? close any open position
	if now.Sub(openTime) >= *flagDipDeadline {
		if h.dipOrder != nil && h.dipOrder.Status.IsOpen() {
			h.dipOrder.Cancel()
			h.dipOrder = nil
		}
		if h.dipShares.IsPositive() {
			// don't sell more than we have (prevent going short)
			sellShares := h.dipShares
			if sellShares.Cmp(h.equity.Quantity) > 0 {
				sellShares = h.equity.Quantity
			}
			if sellShares.IsPositive() {
				limitPrice := bar.Close.Mul(decimal.One.Sub(*flagExitGreed))
				limitPrice = limitPrice.QuantizeNearest(decimal.Cent)
				order, err := h.equity.Order(sellShares.Neg(), limitPrice)
				if err == nil {
					if cubby.Verbose {
						pnl := bar.Close.Sub(h.dipEntryPrice).Div(h.dipEntryPrice).MulInt(10000)
						log.Printf("momo deadline %s: closing %s shares at $%s (pnl %.0f bps)",
							h.equity.Symbol, sellShares, limitPrice, pnl.Float64())
					}
					h.order = order
				}
			}
			h.dipShares = decimal.Zero
			h.dipEntryPrice = decimal.Zero
			h.dipPeakPrice = decimal.Zero
		}
		return
	}

	// manage existing position with trail stop
	if h.dipShares.IsPositive() {
		h.manageDipExit(bar)
		return
	}

	// already have pending order or already traded today
	if h.dipOrder != nil && h.dipOrder.Status.IsOpen() {
		return
	}
	if h.intradayTraded {
		return
	}

	// only look for momentum within the window
	if now.Sub(openTime) > *flagMomoWindow {
		return
	}

	// track highest price and green bars
	if bar.High.Cmp(h.highestPrice) > 0 {
		h.highestPrice = bar.High
	}

	isGreenBar := bar.Close.Cmp(bar.Open) > 0
	if isGreenBar {
		h.greenBarCount++
	} else {
		if h.greenBarCount >= *flagGreenBars {
			h.sawGreenBars = true
		}
		h.greenBarCount = 0
	}

	// check momentum entry conditions
	if !h.openPrice.IsPositive() || !h.sawGreenBars {
		return
	}

	// calculate gain from open
	gainBps := bar.Close.Sub(h.openPrice).Div(h.openPrice).MulInt(10000)
	gainMinBps := flagMomoMin.MulInt(10000)

	// is the gain big enough?
	if gainBps.Cmp(gainMinBps) < 0 {
		return
	}

	// require current bar to be green (continuation)
	if !isGreenBar {
		return
	}

	// calculate position size - pay up slightly for momentum
	limitPrice := bar.Close.Mul(decimal.One.Add(*flagDipGreed))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	maxShares := h.equity.GetMaxOrderQuantity(limitPrice)
	if !maxShares.IsPositive() {
		log.Printf("momo %s: no buying power available (gain %.0f bps)", h.equity.Symbol, gainBps.Float64())
		return
	}

	// cap position size
	portfolioValue := cubby.GetPortfolioValue()
	maxValue := portfolioValue.Mul(*flagDipMaxPct)
	maxPctShares := maxValue.Div(limitPrice).Truncate()
	cappedByPct := maxShares.Cmp(maxPctShares) > 0
	if cappedByPct {
		maxShares = maxPctShares
	}
	if !maxShares.IsPositive() {
		return
	}

	// place order
	order, err := h.equity.Order(maxShares, limitPrice)
	if err != nil {
		log.Printf("momo %s: order rejected: %v (tried %s shares at $%s)",
			h.equity.Symbol, err, maxShares, limitPrice)
		return
	}
	h.dipOrder = order
	h.intradayTraded = true

	capReason := "margin"
	if cappedByPct {
		capReason = fmt.Sprintf("%.0f%% cap", flagDipMaxPct.MulInt(100).Float64())
	}
	log.Printf("momo %s: buying %s shares at $%s (gain %.0f bps, capped by %s)",
		h.equity.Symbol, maxShares, limitPrice, gainBps.Float64(), capReason)
}
