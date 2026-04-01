package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
	"dropbear/options"
	"dropbear/prob"
	"dropbear/symbol"
	"log"
	"math"
	"strings"
	"time"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

type Trader struct {
	Symbol                symbol.Symbol
	Config                *Config
	Chain                 *options.Options
	Holdings              Holdings
	OrderEvents           chan *schwab.OrderEvent
	OrderUpdates          chan OrderUpdate
	OptionsByID           map[uint32]*options.Option
	OptionsByOSI          map[string]*options.Option
	OrdersBySchwabID      map[schwab.OrderID]*Order
	PendingOrders         map[*Order]bool
	PendingOrdersByOption map[*options.Option][]*Order
	StrategiesUsed        map[string]int
	NextTradeTime         clocky.Time
	BaselineRisk          decimal.Decimal
	BaselinePayoff        decimal.Decimal
	BaselineDelta         decimal.Decimal
	BaselineSettlements   []strikeSettlement
	StagedCash            decimal.Decimal
	StagedLegs            []*Leg
	Simulations           *treeset.Set[*Order]
	MarketClose           clocky.Time
	StopTime              clocky.Time
	PanicTime             clocky.Time
	Hinter                *loggy.Hinter
	Web                   *Web
	OrderCounter          int
	IdentifierCounter     int
	StopTransitioned      bool
	AbortOrder            bool
	Panicking             bool
	Paused                bool
}

func NewTrader(symbol symbol.Symbol, config *Config) *Trader {
	return &Trader{
		Symbol:                symbol,
		Config:                config,
		Chain:                 options.NewOptions(),
		Holdings:              Holdings{Positions: map[*options.Option]*Holding{}},
		OrderEvents:           make(chan *schwab.OrderEvent, 64),
		OrderUpdates:          make(chan OrderUpdate, 64),
		PendingOrders:         map[*Order]bool{},
		PendingOrdersByOption: map[*options.Option][]*Order{},
		OrdersBySchwabID:      map[schwab.OrderID]*Order{},
		OptionsByID:           map[uint32]*options.Option{},
		OptionsByOSI:          map[string]*options.Option{},
		StrategiesUsed:        map[string]int{},
		Simulations:           treeset.NewWith(compareOrdersByScore),
		Hinter:                loggy.NewHinter(),
		StopTime:              clocky.MaxTime,
		PanicTime:             clocky.MaxTime,
	}
}

// strikeSettlement caches the baseline portfolio settlement value at a
// given strike. Precomputed once per think cycle so that each order
// only needs to add the delta from its 2 staged legs, turning O(strikes ×
// positions × orders) into O(strikes × orders × 2).
type strikeSettlement struct {
	strike   *options.Strike
	baseline decimal.Decimal // settlement from gPositions only (no cash)
	maxLoss  float64         // interpolated max acceptable loss
}

func (t *Trader) onThought(now clocky.Time) {
	clock := now.ClockInt()
	if clock < t.Config.StartOfDay {
		t.Hinter.Hint("not trading: before market open (%d < %d)", clock, t.Config.StartOfDay)
		return
	}
	if now.After(t.MarketClose) {
		t.Hinter.Hint("not trading: market closed")
		return
	}
	if !t.StopTransitioned && clock >= t.Config.StopTrading {
		t.StopTransitioned = true
		for _, k := range kStrategies {
			t.Config.Strategies[k] = false
		}
		log.Printf("STOPPING TRADING")
	}
	if !t.Panicking && now.After(t.PanicTime) {
		for _, k := range kStrategies {
			t.Config.Strategies[k] = false
			if kStrategyDefaultEOD[k] {
				t.Config.Strategies[k] = true
			}
		}
		if t.Config.NoHurry {
			t.Config.NoHurry = false
			t.Config.Eval = decimal.Two
			t.Config.Spread = decimal.Half.Neg()
			t.PanicTime = t.MarketClose.Add(-t.Config.Panic)
		} else {
			t.Config.AllowClosing = true
			t.Config.Spread = decimal.One
			t.Config.Cooldown = 3 * clocky.Second
			t.Config.Patience = 10 * clocky.Second
			t.Config.BypassPayoff = true
			t.Config.BypassScore = true
			t.Panicking = true
		}
		log.Printf("LIQUIDATING")
	}
	if now < t.NextTradeTime {
		t.Hinter.Hint("not trading: cooldown (%s remaining)", clocky.Duration(t.NextTradeTime-now))
		return
	}
	if len(t.PendingOrders) >= t.Config.MaxPending {
		t.Hinter.Hint("not trading: %d pending orders (max %d)", len(t.PendingOrders), t.Config.MaxPending)
		return
	}
	if t.Paused {
		t.Hinter.Hint("not trading: paused")
		return
	}
	t.trade(now)
}

func (t *Trader) trade(now clocky.Time) {
	if !t.beginOrders() {
		return
	}
	benchStartTime := time.Now()
	em_near := t.Chain.ExpectedMove()
	lo_near := t.Chain.Price.Sub(em_near)
	hi_near := t.Chain.Price.Add(em_near)
	em_wide := t.Chain.ExpectedMove().Mul(t.Config.Sigmas)
	lo_wide := t.Chain.Price.Sub(em_wide)
	hi_wide := t.Chain.Price.Add(em_wide)
	t.Hinter.Hint("price=%s em=%s near=[%s,%s] wide=[%s,%s]", t.Chain.Price, em_near, lo_near, hi_near, lo_wide, hi_wide)
	t.buyPut()
	t.buyCall()
	t.sellPut()
	t.sellCall()
	t.buyCombo(lo_near, hi_near)
	t.sellCombo(lo_near, hi_near)
	t.sellCallVertical(lo_near, hi_wide)
	t.sellPutVertical(lo_wide, hi_near)
	t.buyCallVertical(lo_near, hi_wide)
	t.buyPutVertical(lo_wide, hi_near)
	t.liquidatePut()
	t.liquidateCall()
	t.liquidatePair()
	t.liquidateCallVertical()
	t.liquidatePutVertical()
	t.Hinter.Hint("thought for %s (%d orders)", time.Since(benchStartTime), t.OrderCounter)
	t.sendBestOrder(now)
}

func (t *Trader) beginOrders() bool {
	t.OrderCounter = 0
	t.precomputeSettlements()
	t.BaselinePayoff = t.computeExpectedPayoff()
	if t.BaselinePayoff.Cmp(decimal.Min) == 0 && !t.Panicking {
		t.Hinter.Hint("not thinking: no strikes with positive probability (price=%s em=%s atm=%v)",
			t.Chain.Price, t.Chain.ExpectedMove(), t.Chain.AtTheMoney != nil)
		return false
	}
	t.BaselineRisk = t.computeRisk()
	t.BaselineDelta = t.computeDelta()
	return true
}

func (t *Trader) sendBestOrder(now clocky.Time) {
	t.Hinter.Hint("%d out of %d simulated orders were reasonable",
		t.Simulations.Size(), t.OrderCounter)
	if t.Simulations.Empty() {
		return
	}
	for it := t.Simulations.Iterator(); it.Next(); {
		order := it.Value()
		order.Created = now
		order.ID = t.IdentifierCounter
		t.IdentifierCounter++
		t.NextTradeTime = now.Add(t.Config.Cooldown)
		err := order.Send()
		if err != nil {
			log.Printf("#%d failed to send order: %v", order.ID, err)
			break
		}
		log.Printf("#%d %s: price=%s natural=%s maker=%s score=%s payoff=%s->%s risk=%s->%s",
			order.ID, order.Strategy, order.Price, order.NaturalPrice(), order.MakerPrice(), order.Score,
			t.BaselinePayoff.Format(2), order.Payoff.Format(2), t.BaselineRisk.Format(2), order.Risk.Format(2))
		for _, leg := range order.Legs {
			if leg.Quantity.IsPositive() {
				log.Printf("  buy  %4s  %s", leg.Quantity, leg.Option)
			} else {
				log.Printf("  sell %4s  %s", leg.Quantity.Abs(), leg.Option)
			}
		}
		if true {
			break
		}
	}
	t.Simulations.Clear()
}

// buy simulates buying an option. Aborts if we're already short this
// contract, to prevent churning (buying back what we sold).
func (t *Trader) buy(option *options.Option) {
	t.buyN(option, decimal.One)
}

func (t *Trader) buyN(option *options.Option, qty decimal.Decimal) {
	if !t.Config.AllowClosing && option.Mode == options.ModeShort {
		t.AbortOrder = true
	}
	t.StagedLegs = append(t.StagedLegs, &Leg{
		Option:   option,
		Quantity: qty,
	})
}

// sell simulates selling an option. Aborts if we're already long this
// contract, to prevent churning (selling what we bought).
func (t *Trader) sell(option *options.Option) {
	t.sellN(option, decimal.One)
}

func (t *Trader) sellN(option *options.Option, qty decimal.Decimal) {
	if !t.Config.AllowClosing && option.Mode == options.ModeLong {
		t.AbortOrder = true
	}
	t.StagedLegs = append(t.StagedLegs, &Leg{
		Option:   option,
		Quantity: qty.Neg(),
	})
}

// prune returns true if we should skip this branch of the search.
func (t *Trader) prune() bool {
	return rando() < t.Config.Prune
}

func (t *Trader) end(strategy string) bool {
	defer func() {
		t.StagedLegs = nil
		t.StagedCash = decimal.Zero
		t.AbortOrder = false
	}()
	if t.AbortOrder && !strings.HasPrefix(strategy, "liquidate") {
		return false
	}
	t.OrderCounter++
	price := t.choosePriceForOrder()
	if price.IsZero() {
		return false
	}
	// evaluate at the pessimistic eval spread for scoring
	// this filters out marginal trades that only work at mid
	evalPrice := t.chooseEvalPrice()
	if !t.Panicking && evalPrice.IsZero() {
		return false
	}
	t.StagedCash = evalPrice.MulInt(kMultiplier)
	if !t.Config.BypassRisk && !t.checkRiskTolerance() {
		return false
	}
	payoff := t.computeExpectedPayoff()
	payoffImprovement := payoff.Sub(t.BaselinePayoff)
	if !t.Config.BypassPayoff && payoffImprovement.IsNegative() {
		return false
	}
	risk := t.computeRisk()
	score := t.scoreOrder(payoff, risk)
	if !t.Config.BypassScore && !score.IsPositive() {
		return false
	}
	t.Simulations.Add(&Order{
		Trader:   t,
		ID:       t.OrderCounter,
		Legs:     t.StagedLegs,
		Strategy: strategy,
		Price:    price,
		Risk:     risk,
		Payoff:   payoff,
		Score:    score,
	})
	return true
}

func (t *Trader) scoreOrder(payoff, risk decimal.Decimal) decimal.Decimal {
	floor := decimal.FromFloat64(t.Config.Floor)
	payoffImprovement := payoff.Sub(t.BaselinePayoff)
	riskReduction := t.BaselineRisk.Sub(risk).Div(floor)
	delta := t.computeDelta()
	deltaImprovement := t.BaselineDelta.Abs().Sub(delta.Abs()).Mul(t.Chain.Price).Div(floor)
	a := payoffImprovement.Mul(t.Config.WeightPayoff)
	b := riskReduction.Mul(t.Config.WeightRisk)
	c := deltaImprovement.Mul(t.Config.WeightDelta)
	s := a.Add(b).Add(c)
	// log.Printf("scoring order: payoff=%s improvement=%s weight=%s risk=%s reduction=%s weight=%s delta=%s improvement=%s weight=%s score=%s",
	// 	payoff, payoffImprovement, t.Config.WeightPayoff,
	// 	risk, riskReduction, t.Config.WeightRisk,
	// 	delta, deltaImprovement, t.Config.WeightDelta,
	// 	s)
	return s
}

func (t *Trader) cancelUnfilledOrders(now clocky.Time) {
	t.precomputeSettlements()
	t.BaselinePayoff = t.computeExpectedPayoff()
	for order := range t.PendingOrders {
		if order.Canceling || order.HasFill() || (*liveFlag && order.OrderID == 0) {
			continue
		}
		elapsed := now.Sub(order.Created)
		if elapsed >= t.Config.Patience || t.isPendingOrderToxic(order) {
			err := order.Cancel()
			if err == nil {
				log.Printf("#%d canceling order id %d after %s", order.ID, order.OrderID, elapsed)
			} else {
				log.Printf("#%d failed to cancel order id %d after %s: %v", order.ID, order.OrderID, elapsed, err)
			}
		}
	}
}

// isPendingOrderToxic returns true if market conditions have changed in
// such a way that it's no longer desirable to place an order.
func (t *Trader) isPendingOrderToxic(order *Order) bool {
	t.StagedLegs = order.Legs
	t.StagedCash = order.Price.MulInt(kMultiplier)
	toxic := false
	if !t.Config.BypassRisk && !t.checkRiskTolerance() {
		log.Printf("#%d order is toxic: risk tolerance exceeded", order.ID)
		toxic = true
	}
	newPayoff := t.computeExpectedPayoff()
	payoffImprovement := newPayoff.Sub(t.BaselinePayoff)
	if !t.Config.BypassPayoff && payoffImprovement.IsNegative() {
		log.Printf("#%d order has gone toxic because it would decrease payoff from %s to %s",
			order.ID, t.BaselinePayoff.Format(2), newPayoff.Format(2))
		toxic = true
	}
	t.StagedLegs = nil
	t.StagedCash = decimal.Zero
	return toxic
}

func (t *Trader) choosePriceForOrder() decimal.Decimal {
	return t.choosePriceAtSpread(t.Config.Spread)
}

func (t *Trader) chooseEvalPrice() decimal.Decimal {
	eval := t.Config.Eval
	if eval.IsZero() {
		eval = t.Config.Spread
	}
	return t.choosePriceAtSpread(eval)
}

func (t *Trader) choosePriceAtSpread(spread decimal.Decimal) decimal.Decimal {
	price := decimal.Zero
	for _, leg := range t.StagedLegs {
		opt := leg.Option
		mid := opt.MidPrice()
		hlf := opt.Ask.Sub(opt.Bid).DivInt(2)
		if leg.Quantity.IsPositive() {
			// buying: mid + halfSpread * spread
			price = price.Sub(mid.Add(hlf.Mul(spread)))
		} else {
			// selling: mid - halfSpread * spread
			price = price.Add(mid.Sub(hlf.Mul(spread)))
		}
	}
	// apply our read on penny pilot program rules
	tick, bigTick := getTicks(t.Symbol)
	if len(t.StagedLegs) == 1 && price.Abs().Cmp(kThree) >= 0 {
		tick = bigTick // spreads always quantize on minimum tick size
	}
	// buying is negative (debit) and selling is positive (credit)
	// the ceiling function rounds towards positive infinity
	// therefore we buy low and sell high
	return price.QuantizeCeil(tick)
}

func (t *Trader) onHeartbeat() {
	liq := t.computeLiquidationValue()
	eod := t.computeSettlementAt(t.Chain.Price)
	pay := t.computeExpectedPayoff()
	bad := t.computeRisk()
	delta, gamma, theta, vega := t.computeGreeks()
	log.Printf("cash:%s liq:%s eod:%s worst:%s realized:%s payoff:%s delta:%s gamma:%s theta:%s vega:%s",
		t.Holdings.Cash, liq.Truncate(), eod, bad, t.Holdings.RealizedPnL.Truncate(), pay.Truncate(),
		delta.Format(2), gamma.Format(2), theta.Format(2), vega.Format(2))
}

func (t *Trader) onEndOfDay() {
	log.Printf("end of day settlement time")
	for _, holding := range t.Holdings.Sorted() {
		intrinsic := holding.Option.IntrinsicValue(t.Chain.Price)
		settlement := intrinsic.Mul(holding.Quantity).MulInt(kMultiplier)
		t.Holdings.Cash = t.Holdings.Cash.Add(settlement)
		log.Printf("have %4s of %s worth %8s", holding.Quantity, holding.Option.StringAligned(), settlement.Truncate())
	}
	log.Printf("strategies used")
	for _, s := range kStrategies {
		count := t.StrategiesUsed[s]
		log.Printf("%30s %4d", s, count)
	}
	totalFees := t.Holdings.TotalFees
	log.Printf("ending balance %s less %s fees winning %s at %s price %s",
		t.Holdings.Cash, totalFees, t.Holdings.Cash.Sub(totalFees), t.Symbol, t.Chain.Price)
}

// iteratePositions calls f for all positions.
func (t *Trader) iteratePositions(f func(*options.Option, decimal.Decimal)) {
	for option, position := range t.Holdings.Positions {
		f(option, position.Quantity)
	}
	for _, leg := range t.StagedLegs {
		f(leg.Option, leg.Quantity)
	}
}

// iterateStrikes calls f for each strike within expected move range.
func (t *Trader) iterateStrikes(f func(*options.Strike)) {
	em := t.Chain.ExpectedMove().Mul(t.Config.Sigmas)
	lo := t.Chain.Price.Sub(em)
	hi := t.Chain.Price.Add(em)
	_, strike, _ := t.Chain.Strikes.Floor(lo)
	for strike != nil {
		f(strike)
		if strike.Price.Cmp(hi) >= 0 {
			break
		}
		strike = strike.Next
	}
}

// precomputeSettlements caches baseline settlement values at every strike
// so that orders only need to add the delta from their 2 staged legs.
// riskRamp returns a factor from 0.1 to 1.0 that linearly ramps risk
// tolerance from market open (9:45) to noon (12:00). This prevents the
// robot from maxing out its risk budget in the first few minutes.
func (t *Trader) riskRamp() float64 {
	clock := clocky.Now().ClockInt()
	if clock <= t.Config.StartOfDay {
		return 0.1
	}
	frt := t.Config.FullRiskTime
	if clock >= frt {
		return 1.0
	}
	return 0.1 + 0.9*float64(clock-t.Config.StartOfDay)/float64(frt-t.Config.StartOfDay)
}

func (t *Trader) precomputeSettlements() {
	em := t.Chain.ExpectedMove().Float64()
	t.BaselineSettlements = t.BaselineSettlements[:0]
	for it := t.Chain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		value := decimal.Zero
		for option, holding := range t.Holdings.Positions {
			intrinsic := option.IntrinsicValue(strike.Price)
			value = value.Add(intrinsic.Mul(holding.Quantity))
		}
		ss := strikeSettlement{
			strike:   strike,
			baseline: value.MulInt(kMultiplier),
		}
		if em > 0 {
			movement := t.Chain.Price.Sub(strike.Price).Abs().Float64()
			sigmas := movement / em
			blend := (prob.NormCDF(sigmas) - .5) * 2
			ss.maxLoss = math.FMA(t.Config.Floor-t.Config.Budget, blend, t.Config.Budget) * t.riskRamp()
		}
		t.BaselineSettlements = append(t.BaselineSettlements, ss)
	}
}

// stagedSettlementDelta computes the incremental settlement contribution
// from only the staged positions at the given underlying price.
func (t *Trader) stagedSettlementDelta(underlyingPrice decimal.Decimal) decimal.Decimal {
	value := decimal.Zero
	for _, leg := range t.StagedLegs {
		if !leg.Quantity.IsZero() {
			intrinsic := leg.Option.IntrinsicValue(underlyingPrice)
			value = value.Add(intrinsic.Mul(leg.Quantity))
		}
	}
	return value.MulInt(kMultiplier)
}

// computeRisk calculates the worst possible outcome of the current portfolio.
func (t *Trader) computeRisk() decimal.Decimal {
	worst := decimal.Zero
	for _, ss := range t.BaselineSettlements {
		settlement := ss.baseline.Add(t.Holdings.Cash)
		if len(t.StagedLegs) > 0 {
			settlement = settlement.Add(t.stagedSettlementDelta(ss.strike.Price)).Add(t.StagedCash)
		}
		worst = worst.Min(settlement)
	}
	return worst
}

// checkRiskTolerance verifies that the simulated position's loss at every
// strike is within the acceptable risk tolerance for that probability level.
func (t *Trader) checkRiskTolerance() bool {
	em := t.Chain.ExpectedMove().Float64()
	if em <= 0 {
		return false
	}
	for _, ss := range t.BaselineSettlements {
		settlement := ss.baseline.Add(t.Holdings.Cash)
		if len(t.StagedLegs) > 0 {
			settlement = settlement.Add(t.stagedSettlementDelta(ss.strike.Price)).Add(t.StagedCash)
		}
		if settlement.Float64() < -ss.maxLoss {
			return false
		}
	}
	return true
}

func (t *Trader) computeSettlementAt(underlyingPrice decimal.Decimal) decimal.Decimal {
	value := decimal.Zero
	t.iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		intrinsic := option.IntrinsicValue(underlyingPrice)
		value = value.Add(intrinsic.Mul(pos))
	})
	return value.MulInt(kMultiplier).Add(t.Holdings.Cash).Add(t.StagedCash)
}

func (t *Trader) computeLiquidationValue() decimal.Decimal {
	value := decimal.Zero
	t.iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		mid := option.MidPrice()
		value = value.Add(mid.Mul(pos))
	})
	return value.MulInt(kMultiplier).Add(t.Holdings.Cash).Add(t.StagedCash)
}

// computeNotional returns the total underlying notional value controlled
// by all positions: sum(abs(qty)) * underlying_price * 100.
func (t *Trader) computeNotional() decimal.Decimal {
	var totalQty decimal.Decimal
	t.iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		totalQty = totalQty.Add(pos.Abs())
	})
	return totalQty.Mul(t.Chain.Price).MulInt(kMultiplier)
}

func (t *Trader) computeExpectedPayoff() decimal.Decimal {
	// collect probabilities from cached settlements
	type cachedProb struct {
		idx  int
		prob decimal.Decimal
	}
	em := t.Chain.ExpectedMove().Mul(t.Config.Sigmas)
	lo := t.Chain.Price.Sub(em)
	hi := t.Chain.Price.Add(em)
	var probs []cachedProb
	var probSum decimal.Decimal
	for i, ss := range t.BaselineSettlements {
		if ss.strike.Price.Cmp(lo) < 0 || ss.strike.Price.Cmp(hi) > 0 {
			continue
		}
		p := ss.strike.Probability()
		if p.IsPositive() {
			probs = append(probs, cachedProb{i, p})
			probSum = probSum.Add(p)
		}
	}
	if len(probs) == 0 || !probSum.IsPositive() {
		return decimal.Min
	}
	// compute expected payoff using cached baselines
	payoff := decimal.Zero
	for _, cp := range probs {
		ss := &t.BaselineSettlements[cp.idx]
		settlement := ss.baseline
		if len(t.StagedLegs) > 0 {
			settlement = settlement.Add(t.stagedSettlementDelta(ss.strike.Price))
		}
		payoff = payoff.Add(settlement.Mul(cp.prob.Div(probSum)))
	}
	return payoff.Add(t.Holdings.Cash).Add(t.StagedCash)
}

// computeBias returns what direction options market thinks underlying will move.
func (t *Trader) computeBias() decimal.Decimal {
	atm := t.Chain.AtTheMoney
	if atm == nil || !atm.IsReady() {
		return decimal.Zero
	}
	return atm.Call.MidPrice().Sub(atm.Put.MidPrice()).MulInt(kMultiplier)
}

// computeDelta calculates the delta for all positions.
func (t *Trader) computeDelta() decimal.Decimal {
	delta := decimal.Zero
	t.iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		delta = delta.Add(option.Delta.Mul(pos))
	})
	return delta.MulInt(kMultiplier)
}

func (t *Trader) computeGreeks() (delta, gamma, theta, vega decimal.Decimal) {
	t.iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		delta = delta.Add(option.Delta.Mul(pos))
		gamma = gamma.Add(option.Gamma.Mul(pos))
		theta = theta.Add(option.Theta.Mul(pos))
		vega = vega.Add(option.Vega.Mul(pos))
	})
	delta = delta.MulInt(kMultiplier)
	gamma = gamma.MulInt(kMultiplier)
	theta = theta.MulInt(kMultiplier)
	vega = vega.MulInt(kMultiplier)
	return
}

func (t *Trader) onOptionDef(o *options.Option) {
	osi := o.OSI()
	if existing := t.OptionsByOSI[osi]; existing != nil {
		existing.ID = o.ID
		o = existing
	}
	t.OptionsByID[o.ID] = o
	t.OptionsByOSI[osi] = o
}

func (t *Trader) onOptionDefEnd() {

	// get close time
	now := clocky.Now()
	year, month, day := now.Date()
	t.MarketClose = cboe.GetCloseTime(year, month, day)

	// compute panic time
	if t.Config.Panic > 0 {
		t.PanicTime = t.MarketClose.Add(-t.Config.Panic)
	}

	// initialize options restrictions after loading schwab order history
	for option, holding := range t.Holdings.Positions {
		if holding.Quantity.IsPositive() {
			option.Mode = options.ModeLong
		} else if holding.Quantity.IsNegative() {
			option.Mode = options.ModeShort
		}
	}

	// ensure puts and calls at same strike have opposite restriction
	for it := t.Chain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		switch strike.Call.Mode {
		case options.ModeNone:
			if strike.Put.Mode != options.ModeNone {
				strike.Call.Mode = strike.Put.Mode.Invert()
			}
		case options.ModeLong:
			if strike.Put.Mode == options.ModeNone {
				strike.Put.Mode = options.ModeShort
			}
		case options.ModeShort:
			if strike.Put.Mode == options.ModeNone {
				strike.Put.Mode = options.ModeLong
			}
		}
	}

	// ensure at-the-money has a restriction
	if t.Chain.AtTheMoney.Call.Mode == options.ModeNone {
		t.Chain.AtTheMoney.Call.Mode = options.ModeShort
		t.Chain.AtTheMoney.Put.Mode = options.ModeLong
	}

	// now color remaining strikes as checkered as possible
	for s := t.Chain.AtTheMoney.Next; s != nil; s = s.Next {
		if s.Call.Mode == options.ModeNone {
			s.Call.Mode = s.Prev.Call.Mode.Invert()
		}
		if s.Put.Mode == options.ModeNone {
			s.Put.Mode = s.Prev.Put.Mode.Invert()
		}
	}
	for s := t.Chain.AtTheMoney.Prev; s != nil; s = s.Prev {
		if s.Call.Mode == options.ModeNone {
			s.Call.Mode = s.Next.Call.Mode.Invert()
		}
		if s.Put.Mode == options.ModeNone {
			s.Put.Mode = s.Next.Put.Mode.Invert()
		}
	}
}

func (t *Trader) onOptionTick(m *databento.CMBP1) *options.Option {
	o := t.OptionsByID[m.Header.InstrumentID]
	if o != nil {
		switch m.Action {
		case databento.ActionTrade:
			t.onOptionTrade(o, m)
		case databento.ActionAdd:
			if m.Header.TSEvent > o.TS {
				t.onOptionQuote(o, m)
			}
		default:
			panic("unexpected option tick action: " + m.Action.String())
		}
	}
	return o
}

func (t *Trader) onOptionTrade(o *options.Option, m *databento.CMBP1) {
}

func (t *Trader) onOptionQuote(o *options.Option, m *databento.CMBP1) {
	o.TS = m.Header.TSEvent
	mustRecomputeGreeks := false
	bid := m.Levels[0].BidPx
	if bid != databento.UndefPrice {
		price := decimal.Decimal(bid / 1000)
		if price.Cmp(o.Bid) != 0 {
			mustRecomputeGreeks = true
		}
		o.Bid = price
		o.BidSize = m.Levels[0].BidSz
		o.Got |= options.GotBid
	} else {
		o.Bid = decimal.Zero
		o.BidSize = 0
		o.Got &^= options.GotBid
	}
	ask := m.Levels[0].AskPx
	if ask != databento.UndefPrice {
		price := decimal.Decimal(ask / 1000)
		if price.Cmp(o.Ask) != 0 {
			mustRecomputeGreeks = true
		}
		o.Ask = price
		o.AskSize = m.Levels[0].AskSz
		o.Got |= options.GotAsk
	} else {
		o.Ask = decimal.Zero
		o.AskSize = 0
		o.Got &^= options.GotAsk
	}
	if t.Chain.Add(o) {
		mustRecomputeGreeks = true
	}
	if mustRecomputeGreeks {
		o.ComputeGreeks(t.Chain.Price, kRiskFreeRate, decimal.Zero)
	}
}
