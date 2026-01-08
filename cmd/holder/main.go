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
	flagBenchmark     = flag.String("benchmark", "QQQ", "benchmark symbol")
	flagMomentum      = flag.Int("momentum", 20, "momentum lookback period in bars")
	flagRebalance     = flag.Int("rebalance", 30, "minutes between rebalance checks")
	flagDayLeverage   = decimal.Flag("day-leverage", "4", "max intraday leverage")
	flagNightLeverage = decimal.Flag("night-leverage", "1.5", "overnight leverage (leave room for day trading)")
	flagMinWeight     = decimal.Flag("min-weight", "0.1", "minimum weight per symbol (0-1)")
)

const (
	openTime              = 6_30_00
	normalCloseTime       = 13_00_00
	normalMarginCheckTime = 12_50_00
	earlyCloseTime        = 10_00_00
	earlyMarginCheckTime  = 9_50_00
)

var earlyCloseDates = map[int]bool{
	// July 3rd (or nearest trading day before July 4th)
	// Day after Thanksgiving (4th Friday in November)
	// Christmas Eve (Dec 24, or nearest trading day)
	20200703: true, 20201127: true, 20201224: true,
	20210702: true, 20211126: true, 20211224: true,
	20220701: true, 20221125: true, 20221223: true,
	20230703: true, 20231124: true, 20231222: true,
	20240703: true, 20241129: true, 20241224: true,
	20250703: true, 20251128: true, 20251224: true,
	20260703: true, 20261127: true, 20261224: true,
	20270702: true, 20271126: true, 20271224: true,
}

var (
	flagSlipClose = decimal.FlagBPS("slip-close", "50", "slippage on close orders")
	flagUrgency   = decimal.FlagBPS("urgency", "100", "extra slippage per minute when closing late")
)

var (
	gHoldings        []*Holding
	gDate            int
	gCloseTime       int
	gMarginCheckTime int
	gLastRebalance   clocky.Time
	gTotalMomentum   decimal.Decimal
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

	benchSym := symbol.MustParse(*flagBenchmark)
	benchEquity, _ := cubby.AddEquity(benchSym)
	cubby.Benchmark = benchEquity

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
		if earlyCloseDates[date] {
			gCloseTime = earlyCloseTime
			gMarginCheckTime = earlyMarginCheckTime
		} else {
			gCloseTime = normalCloseTime
			gMarginCheckTime = normalMarginCheckTime
		}
	}
	if date != h.lastDate {
		h.lastDate = date
		h.barCount = 0
	}

	if cubby.IsWarmingUp {
		return
	}

	// update momentum indicator
	h.momentum.Add(bar.Close)
	h.barCount++

	// check order status
	if h.order != nil && h.order.Status.IsFinal() {
		h.order = nil
	}

	now := clocky.Now()
	time := now.ClockInt()

	if time < openTime || time >= gCloseTime {
		return
	}

	// before close: aggressively reduce to overnight leverage
	if time >= gMarginCheckTime {
		h.reduceForOvernight(time)
		return
	}

	// during day: rebalance periodically to day leverage
	if now.Sub(gLastRebalance) >= clocky.Duration(*flagRebalance)*clocky.Minute {
		gLastRebalance = now
		updateWeights()
		h.rebalanceToLeverage(*flagDayLeverage)
	}
}

// updateWeights calculates momentum-weighted target allocations
func updateWeights() {
	n := len(gHoldings)
	equalWeight := decimal.One.DivInt(n)

	// calculate total positive momentum
	gTotalMomentum = decimal.Zero
	for _, h := range gHoldings {
		if h.momentum.IsReady() && h.momentum.Value.IsPositive() {
			gTotalMomentum = gTotalMomentum.Add(h.momentum.Value)
		}
	}

	// clamp minWeight so that minWeight * n <= 0.5 (leave room for momentum)
	minWeight := (*flagMinWeight).Min(decimal.Parse("0.5").DivInt(n))
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

	// place order
	var limitPrice decimal.Decimal
	if diff.IsPositive() {
		// buying: pay slightly above market
		limitPrice = h.equity.Price.Mul(decimal.Parse("1.0025"))
	} else {
		// selling: accept slightly below market
		limitPrice = h.equity.Price.Mul(decimal.Parse("0.9975"))
	}
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

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
func (h *Holding) reduceForOvernight(time int) {
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
	totalWindow := gCloseTime - gMarginCheckTime
	elapsed := time - gMarginCheckTime
	urgency := decimal.Zero
	if totalWindow > 0 && elapsed > 0 {
		urgency = decimal.FromInt(elapsed).Div(decimal.FromInt(totalWindow))
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
