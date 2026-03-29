package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/black76"
	"dropbear/ds/options"
	"dropbear/ds/prob"
	"dropbear/ds/symbol"
	"dropbear/loggy"
	"flag"
	"log"
	"maps"
	"math"
	"os"
	"os/signal"
	"runtime/pprof"
	"slices"
	"strings"
	"time"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

var (
	allFlag        = flag.Bool("all", false, "enable all strategies")
	noneFlag       = flag.Bool("none", false, "disable all strategies")
	noliqFlag      = flag.Bool("noliq", false, "disable liquidation strategies")
	comboFlag      = flag.Bool("combo", false, "enable buying and selling combos")
	hostileFlag    = flag.Bool("hostile", false, "simulate maximum hostility on fills")
	eodFlag        = flag.Bool("eod", false, "enable end-of-day liquidation strategies")
	dbnFlag        = flag.String("dbn", "", "path to backtest data")
	liveFlag       = flag.Bool("live", false, "run in live trading mode")
	webFlag        = flag.Bool("web", false, "enable web dashboard feature")
	rtFlag         = flag.Bool("rt", false, "run backtest in real time mode")
	dryFlag        = flag.Bool("dry", false, "don't send new orders in live mode")
	symbolFlag     = flag.String("symbol", "XSP", "symbol to trade (e.g. XSP, SPXW)")
	dateFlag       = clocky.TimeFlag("date", "2026-03-19", "date of the trades to report")
	sigmasFlag     = decimal.Flag("sigmas", "2", "number of sigmas of strikes to consider")
	budgetFlag     = flag.Float64("budget", 5_000, "maximum acceptable loss at current price")
	floorFlag      = flag.Float64("floor", 40_000, "maximum acceptable loss in catastrophic scenario")
	spreadFlag     = decimal.Flag("spread", "0", "spread crossing (-1=make, 0=mid, 1=take)")
	pruneFlag      = flag.Float64("prune", .5, "probability of stochastic pruning in strategy search")
	closeFlag      = flag.Float64("closer", .1, "random probability that closing will be allowed")
	demandFlag     = decimal.Flag("demand", "1.1", "minimum ratio of open credit to close cost for liquidation")
	wPayoffFlag    = decimal.Flag("w-payoff", "1", "scoring weight for expected payoff improvement")
	wRiskFlag      = decimal.Flag("w-risk", "1", "scoring weight for risk reduction")
	wDeltaFlag     = decimal.Flag("w-delta", "1", "scoring weight for delta neutrality improvement")
	maxErrorFlag   = decimal.Flag("max-error", "1", "maximum acceptable accounting error before panic")
	thinkFlag      = clocky.DurationFlag("think", "50ms", "interval between backtest trading analysis")
	patienceFlag   = clocky.DurationFlag("patience", "10s", "how long to wait before canceling live orders")
	cooldownFlag   = clocky.DurationFlag("cooldown", "10s", "interval between trading decisions")
	slowdownFlag   = clocky.DurationFlag("slowdown", "200ms", "polling limit for web dashboard")
	heartbeatFlag  = clocky.DurationFlag("heartbeat", "1m", "interval between status reports")
	maxPendingFlag = flag.Int("max-pending", 1, "maximum number of pending orders")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
)

const (
	kStartOfDay   = 9_30_00
	kFullRiskTime = 12_00_00
	kStopTrading  = 13_00_00
	kMarketClose  = 16_00_00
	kMultiplier   = 100
)

var (
	gChain               = options.NewOptions()
	gSymbol              symbol.Symbol
	gHoldings            = Holdings{Positions: map[*options.Option]*Holding{}}
	gOptionsByID         = map[uint32]*options.Option{}
	gOptionsByOSI        = map[string]*options.Option{}
	gStrategiesUsed      = map[string]int{}
	gNextTradeTime       clocky.Time
	gBaselineRisk        decimal.Decimal
	gBaselinePayoff      decimal.Decimal
	gBaselineDelta       decimal.Decimal
	gBaselineSettlements []strikeSettlement // precomputed per think cycle
)

// strikeSettlement caches the baseline portfolio settlement value at a
// given strike. Precomputed once per think cycle so that each order
// only needs to add the delta from its 2 staged legs, turning O(strikes ×
// positions × orders) into O(strikes × orders × 2).
type strikeSettlement struct {
	strike   *options.Strike
	baseline decimal.Decimal // settlement from gPositions only (no cash)
	maxLoss  float64         // interpolated max acceptable loss
}

var (
	gStagedCash        decimal.Decimal
	gStagedLegs        []*Leg
	gSimulations       = treeset.NewWith(compareOrders)
	gOrderCounter      int
	gIdentifierCounter int
	gAbortOrder        bool
	gEODTransitioned   bool
	gAllowClosing      bool
)

var (
	kFeePerContract = decimal.Parse("1.2")
	kRiskFreeRate   = decimal.Parse("0.035")
)

func main() {
	loggy.Init()
	flag.Parse()
	log.Println(strings.Join(os.Args, " "))
	black76.DaysPerYear = 252
	black76.HoursPerDay = 6.5
	gSymbol = symbol.MustParse(*symbolFlag)
	if *noneFlag {
		for k := range gStrategyEnabled {
			gStrategyEnabled[k] = false
		}
	}
	if *comboFlag {
		gStrategyEnabled[kStrategyBuyCombo] = true
		gStrategyEnabled[kStrategySellCombo] = true
		gStrategyEnabledEOD[kStrategyLiquidateCall] = true
		gStrategyEnabledEOD[kStrategyLiquidatePut] = true
	}
	if *allFlag {
		for k := range gStrategyEnabled {
			gStrategyEnabled[k] = true
		}
	}
	if *noliqFlag {
		gStrategyEnabled[kStrategyLiquidateCall] = false
		gStrategyEnabled[kStrategyLiquidatePut] = false
		gStrategyEnabled[kStrategyLiquidateCallVertical] = false
		gStrategyEnabled[kStrategyLiquidatePutVertical] = false
	}
	if *webFlag {
		startWeb()
	}
	if *flagCPUProfile != "" {
		f, err := os.Create(*flagCPUProfile)
		if err != nil {
			log.Fatalf("could not create CPU profile: %v", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatalf("could not start CPU profile: %v", err)
		}
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			<-c
			pprof.StopCPUProfile()
			f.Close()
			os.Exit(0)
		}()
		defer pprof.StopCPUProfile()
		defer f.Close()
	}
	if *liveFlag {
		live()
	} else {
		backtest()
	}
}

func onThought(now clocky.Time) {
	clock := now.ClockInt()
	if clock < kStartOfDay {
		loggy.Hint("not trading: before market open (%d < %d)", clock, kStartOfDay)
		return
	}
	if now < gNextTradeTime {
		loggy.Hint("not trading: cooldown (%s remaining)", clocky.Duration(gNextTradeTime-now))
	} else if clock >= kMarketClose {
		loggy.Hint("not trading: market closed (%d >= %d)", clock, kMarketClose)
	} else if len(gPendingOrders) >= *maxPendingFlag {
		loggy.Hint("not trading: %d pending orders (max %d)", len(gPendingOrders), *maxPendingFlag)
	} else if gPaused {
		loggy.Hint("not trading: paused")
	} else {
		trade(now)
	}
}

func trade(now clocky.Time) {
	if !beginOrders(now) {
		return
	}
	benchStartTime := time.Now()
	em_near := gChain.ExpectedMove().DivInt(2)
	lo_near := gChain.Price.Sub(em_near)
	hi_near := gChain.Price.Add(em_near)
	em_wide := gChain.ExpectedMove().Mul(*sigmasFlag)
	lo_wide := gChain.Price.Sub(em_wide)
	hi_wide := gChain.Price.Add(em_wide)
	loggy.Hint("price=%s em=%s near=[%s,%s] wide=[%s,%s]", gChain.Price, em_near, lo_near, hi_near, lo_wide, hi_wide)
	buyPut()
	buyCall()
	sellPut()
	sellCall()
	buyCombo(lo_near, hi_near)
	sellCombo(lo_near, hi_near)
	sellCallVertical(lo_near, hi_wide)
	sellPutVertical(lo_wide, hi_near)
	buyCallVertical(lo_near, hi_wide)
	buyPutVertical(lo_wide, hi_near)
	liquidatePut()
	liquidateCall()
	liquidatePair()
	liquidateCallVertical()
	liquidatePutVertical()
	loggy.Hint("thought for %s (%d orders)", time.Since(benchStartTime), gOrderCounter)
	sendBestOrder(now)
}

func beginOrders(now clocky.Time) bool {
	gOrderCounter = 0
	precomputeSettlements()
	gBaselinePayoff = computeExpectedPayoff()
	if gBaselinePayoff.Cmp(decimal.Min) == 0 {
		loggy.Hint("not thinking: no strikes with positive probability (price=%s em=%s atm=%v)",
			gChain.Price, gChain.ExpectedMove(), gChain.AtTheMoney != nil)
		return false
	}
	gBaselineRisk = computeRisk()
	gBaselineDelta = computeDelta()
	if !gEODTransitioned && *eodFlag {
		isPowerHour := now.ClockInt() >= kStopTrading
		if isPowerHour {
			pay := gBaselinePayoff
			liq := computeLiquidationValue()
			eod := computeSettlementAt(gChain.Price)
			if liq.Cmp(pay) < 0 {
				loggy.Hint("not liquidating: liquidation value %s is less than payoff %s", liq.Truncate(), pay.Truncate())
			} else if pay.Cmp(decimal.FromInt(3000)) < 0 {
				loggy.Hint("not liquidating: payoff %s is too low", pay.Truncate())
			} else if liq.Cmp(eod.DivInt(2)) < 0 {
				loggy.Hint("not liquidating: not enough theta gained; liquidation value %s < 50%% of payoff %s", liq.Truncate(), eod.Truncate())
			} else {
				clear(gStrategyEnabled)
				maps.Copy(gStrategyEnabled, gStrategyEnabledEOD)
				*patienceFlag = 10 * clocky.Second
				*cooldownFlag = 3 * clocky.Second
				gEODTransitioned = true
				gAllowClosing = true
				log.Printf("LIQUIDATION MODE ENABLED")
			}
		}
	}
	return true
}

func sendBestOrder(now clocky.Time) {
	if gSimulations.Empty() {
		return
	}
	loggy.Hint("%d out of %d simulated orders were reasonable",
		gSimulations.Size(), gOrderCounter)
	for it := gSimulations.Iterator(); it.Next(); {
		order := it.Value()
		order.Created = now
		order.ID = gIdentifierCounter
		gIdentifierCounter++
		gNextTradeTime = now.Add(*cooldownFlag)
		err := order.Send()
		if err != nil {
			loggy.Hint("#%d failed to send order: %v", order.ID, err)
			break
		}
		log.Printf("#%d %s: price=%s natural=%s maker=%s score=%s payoff=%s->%s risk=%s->%s",
			order.ID, order.Strategy, order.Price, order.NaturalPrice(), order.MakerPrice(), order.Score,
			gBaselinePayoff.Format(2), order.Payoff.Format(2), gBaselineRisk.Format(2), order.Risk.Format(2))
		for _, leg := range order.Legs {
			if leg.Quantity.IsPositive() {
				log.Printf("  buy  %s", leg.Option)
			} else {
				log.Printf("  sell %s", leg.Option)
			}
		}
		if true {
			break
		}
	}
	gSimulations.Clear()
}

// buy simulates buying an option. Aborts if we're already short this
// contract, to prevent churning (buying back what we sold).
func buy(option *options.Option) {
	if !gAllowClosing && option.Mode == options.ModeShort {
		gAbortOrder = true
		return
	}
	gStagedLegs = append(gStagedLegs, &Leg{
		Option:   option,
		Quantity: decimal.One,
	})
}

// sell simulates selling an option. Aborts if we're already long this
// contract, to prevent churning (selling what we bought).
func sell(option *options.Option) {
	if !gAllowClosing && option.Mode == options.ModeLong {
		gAbortOrder = true
		return
	}
	gStagedLegs = append(gStagedLegs, &Leg{
		Option:   option,
		Quantity: decimal.NegOne,
	})
}

// prune returns true if we should skip this branch of the search.
func prune() bool {
	return rando() < *pruneFlag
}

func end(strategy string) bool {
	defer func() {
		gStagedLegs = nil
		gStagedCash = decimal.Zero
	}()
	if gAbortOrder {
		gAbortOrder = false
		return false
	}
	gOrderCounter++
	price := choosePriceForOrder()
	if price.IsZero() {
		return false
	}
	gStagedCash = price.MulInt(kMultiplier)
	if !checkRiskTolerance() {
		return false
	}
	payoff := computeExpectedPayoff()
	// payoffImprovement := payoff.Sub(gBaselinePayoff)
	// if payoffImprovement.IsNegative() {
	// 	return false
	// }
	risk := computeRisk()
	score := scoreOrder(payoff, risk)
	if !score.IsPositive() {
		return false
	}
	gSimulations.Add(&Order{
		ID:       gOrderCounter,
		Legs:     gStagedLegs,
		Strategy: strategy,
		Price:    price,
		Risk:     risk,
		Payoff:   payoff,
		Score:    score,
	})
	return true
}

func scoreOrder(payoff, risk decimal.Decimal) decimal.Decimal {
	payoffImprovement := payoff.Sub(gBaselinePayoff)
	riskReduction := risk.Sub(gBaselineRisk).Max(decimal.Zero)
	delta := computeDelta()
	deltaImprovement := gBaselineDelta.Abs().Sub(delta.Abs())
	a := payoffImprovement.Mul(*wPayoffFlag)
	b := riskReduction.Mul(*wRiskFlag)
	c := deltaImprovement.Mul(*wDeltaFlag)
	return a.Add(b).Add(c)
}

func cancelUnfilledOrders(now clocky.Time) {
	precomputeSettlements()
	gBaselinePayoff = computeExpectedPayoff()
	for order := range gPendingOrders {
		if order.Canceling {
			continue
		}
		elapsed := now.Sub(order.Created)
		if elapsed >= *patienceFlag || isPendingOrderToxic(order) {
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
func isPendingOrderToxic(order *Order) bool {
	gStagedLegs = order.Legs
	gStagedCash = order.Price.MulInt(kMultiplier)
	toxic := false
	if !checkRiskTolerance() {
		log.Printf("#%d order is toxic: risk tolerance exceeded", order.ID)
		toxic = true
	}
	newPayoff := computeExpectedPayoff()
	payoffImprovement := newPayoff.Sub(gBaselinePayoff)
	if payoffImprovement.IsNegative() {
		log.Printf("#%d order has gone toxic because it would decrease payoff from %s to %s",
			order.ID, gBaselinePayoff.Format(2), newPayoff.Format(2))
		toxic = true
	}
	gStagedLegs = nil
	gStagedCash = decimal.Zero
	return toxic
}

func choosePriceForOrder() decimal.Decimal {
	// exec controls where in the spread we price:
	//   -1 = make (bid for buys, ask for sells — most favorable)
	//    0 = mid
	//    1 = take (ask for buys, bid for sells — least favorable)
	// values beyond ±1 go past the spread
	spread := *spreadFlag
	price := decimal.Zero
	for _, leg := range gStagedLegs {
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
	tick, bigTick := getTicks(gSymbol)
	if len(gStagedLegs) == 1 && price.Abs().Cmp(kThree) >= 0 {
		tick = bigTick // spreads always quantize on minimum tick size
	}
	// buying is negative (debit) and selling is positive (credit)
	// the ceiling function rounds towards positive infinity
	// therefore we buy low and sell high
	return price.QuantizeCeil(tick)
}

func onHeartbeat() {
	liq := computeLiquidationValue()
	eod := computeSettlementAt(gChain.Price)
	pay := computeExpectedPayoff()
	bad := computeRisk()
	delta, gamma, theta, vega := computeGreeks()
	log.Printf("cash:%s liq:%s eod:%s worst:%s realized:%s payoff:%s delta:%s gamma:%s theta:%s vega:%s",
		gHoldings.Cash, liq.Truncate(), eod, bad, gHoldings.RealizedPnL.Truncate(), pay.Truncate(),
		delta.Format(2), gamma.Format(2), theta.Format(2), vega.Format(2))
}

func onEndOfDay() {
	log.Printf("end of day settlement time")
	iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		intrinsic := option.IntrinsicValue(gChain.Price)
		settlement := intrinsic.Mul(pos).MulInt(kMultiplier)
		gHoldings.Cash = gHoldings.Cash.Add(settlement)
		log.Printf("have %4s of %s worth %8s", pos, option.StringAligned(), settlement.Truncate())
	})
	log.Printf("strategies used")
	for _, strategy := range slices.Sorted(maps.Keys(gStrategiesUsed)) {
		count := gStrategiesUsed[strategy]
		log.Printf("%30s %4d", strategy, count)
	}
	totalFees := gHoldings.TotalFees
	log.Printf("ending balance %s less %s fees winning %s at %s price %s",
		gHoldings.Cash, totalFees, gHoldings.Cash.Sub(totalFees), gSymbol, gChain.Price)
}

// iteratePositions calls f for all positions.
func iteratePositions(f func(*options.Option, decimal.Decimal)) {
	for option, position := range gHoldings.Positions {
		f(option, position.Quantity)
	}
	for _, leg := range gStagedLegs {
		f(leg.Option, leg.Quantity)
	}
}

// iterateStrikes calls f for each strike within expected move range.
func iterateStrikes(f func(*options.Strike)) {
	em := gChain.ExpectedMove().Mul(*sigmasFlag)
	lo := gChain.Price.Sub(em)
	hi := gChain.Price.Add(em)
	_, strike, _ := gChain.Strikes.Floor(lo)
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
func riskRamp() float64 {
	const rampStart = kStartOfDay
	const rampEnd = kFullRiskTime
	clock := float64(clocky.Now().ClockInt())
	if clock <= rampStart {
		return 0.1
	}
	if clock >= rampEnd {
		return 1.0
	}
	return 0.1 + 0.9*(clock-rampStart)/(rampEnd-rampStart)
}

func precomputeSettlements() {
	em := gChain.ExpectedMove().Float64()
	gBaselineSettlements = gBaselineSettlements[:0]
	for it := gChain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		value := decimal.Zero
		for option, holding := range gHoldings.Positions {
			intrinsic := option.IntrinsicValue(strike.Price)
			value = value.Add(intrinsic.Mul(holding.Quantity))
		}
		ss := strikeSettlement{
			strike:   strike,
			baseline: value.MulInt(kMultiplier),
		}
		if em > 0 {
			movement := gChain.Price.Sub(strike.Price).Abs().Float64()
			sigmas := movement / em
			blend := (prob.NormCDF(sigmas) - .5) * 2
			ss.maxLoss = math.FMA(*floorFlag-*budgetFlag, blend, *budgetFlag) * riskRamp()
		}
		gBaselineSettlements = append(gBaselineSettlements, ss)
	}
}

// stagedSettlementDelta computes the incremental settlement contribution
// from only the staged positions at the given underlying price.
func stagedSettlementDelta(underlyingPrice decimal.Decimal) decimal.Decimal {
	value := decimal.Zero
	for _, leg := range gStagedLegs {
		if !leg.Quantity.IsZero() {
			intrinsic := leg.Option.IntrinsicValue(underlyingPrice)
			value = value.Add(intrinsic.Mul(leg.Quantity))
		}
	}
	return value.MulInt(kMultiplier)
}

// computeRisk calculates the worst possible outcome of the current portfolio.
func computeRisk() decimal.Decimal {
	worst := decimal.Zero
	for _, ss := range gBaselineSettlements {
		settlement := ss.baseline.Add(gHoldings.Cash)
		if len(gStagedLegs) > 0 {
			settlement = settlement.Add(stagedSettlementDelta(ss.strike.Price)).Add(gStagedCash)
		}
		worst = worst.Min(settlement)
	}
	return worst
}

// checkRiskTolerance verifies that the simulated position's loss at every
// strike is within the acceptable risk tolerance for that probability level.
func checkRiskTolerance() bool {
	em := gChain.ExpectedMove().Float64()
	if em <= 0 {
		return false
	}
	for _, ss := range gBaselineSettlements {
		settlement := ss.baseline.Add(gHoldings.Cash)
		if len(gStagedLegs) > 0 {
			settlement = settlement.Add(stagedSettlementDelta(ss.strike.Price)).Add(gStagedCash)
		}
		if settlement.Float64() < -ss.maxLoss {
			return false
		}
	}
	return true
}

func computeSettlementAt(underlyingPrice decimal.Decimal) decimal.Decimal {
	value := decimal.Zero
	iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		intrinsic := option.IntrinsicValue(underlyingPrice)
		value = value.Add(intrinsic.Mul(pos))
	})
	return value.MulInt(kMultiplier).Add(gHoldings.Cash).Add(gStagedCash)
}

// computeRiskSlow is the non-cached version for the dashboard.
func computeRiskSlow() decimal.Decimal {
	worst := decimal.Zero
	for it := gChain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		settlement := computeSettlementAt(strike.Price)
		worst = worst.Min(settlement)
	}
	return worst
}

func computeLiquidationValue() decimal.Decimal {
	value := decimal.Zero
	iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		mid := option.MidPrice()
		value = value.Add(mid.Mul(pos))
	})
	return value.MulInt(kMultiplier).Add(gHoldings.Cash).Add(gStagedCash)
}

// computeNotional returns the total underlying notional value controlled
// by all positions: sum(abs(qty)) * underlying_price * 100.
func computeNotional() decimal.Decimal {
	var totalQty decimal.Decimal
	iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		totalQty = totalQty.Add(pos.Abs())
	})
	return totalQty.Mul(gChain.Price).MulInt(kMultiplier)
}

func computeExpectedPayoff() decimal.Decimal {
	// collect probabilities from cached settlements
	type cachedProb struct {
		idx  int
		prob decimal.Decimal
	}
	em := gChain.ExpectedMove().Mul(*sigmasFlag)
	lo := gChain.Price.Sub(em)
	hi := gChain.Price.Add(em)
	var probs []cachedProb
	var probSum decimal.Decimal
	for i, ss := range gBaselineSettlements {
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
		ss := &gBaselineSettlements[cp.idx]
		settlement := ss.baseline
		if len(gStagedLegs) > 0 {
			settlement = settlement.Add(stagedSettlementDelta(ss.strike.Price))
		}
		payoff = payoff.Add(settlement.Mul(cp.prob.Div(probSum)))
	}
	return payoff.Add(gHoldings.Cash).Add(gStagedCash)
}

// computeBias returns what direction options market thinks underlying will move.
func computeBias() decimal.Decimal {
	atm := gChain.AtTheMoney
	if atm == nil || !atm.IsReady() {
		return decimal.Zero
	}
	return atm.Call.MidPrice().Sub(atm.Put.MidPrice()).MulInt(kMultiplier)
}

// computeDelta calculates the delta for all positions.
func computeDelta() decimal.Decimal {
	delta := decimal.Zero
	iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		delta = delta.Add(option.Delta.Mul(pos))
	})
	return delta.MulInt(kMultiplier)
}

func computeGreeks() (delta, gamma, theta, vega decimal.Decimal) {
	iteratePositions(func(option *options.Option, pos decimal.Decimal) {
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

func onOptionDef(o *options.Option) {
	gOptionsByID[o.ID] = o
	gOptionsByOSI[o.OSI()] = o
}

func onOptionDefEnd() {

	// initialize options restrictions after loading schwab order history
	for option, holding := range gHoldings.Positions {
		if holding.Quantity.IsPositive() {
			option.Mode = options.ModeLong
		} else if holding.Quantity.IsNegative() {
			option.Mode = options.ModeShort
		}
	}

	// ensure puts and calls at same strike have opposite restriction
	for it := gChain.Strikes.Iterator(); it.Next(); {
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
	if gChain.AtTheMoney.Call.Mode == options.ModeNone {
		gChain.AtTheMoney.Call.Mode = options.ModeShort
		gChain.AtTheMoney.Put.Mode = options.ModeLong
	}

	// now color remaining strikes as checkered as possible
	for s := gChain.AtTheMoney.Next; s != nil; s = s.Next {
		if s.Call.Mode == options.ModeNone {
			s.Call.Mode = s.Prev.Call.Mode.Invert()
		}
		if s.Put.Mode == options.ModeNone {
			s.Put.Mode = s.Prev.Put.Mode.Invert()
		}
	}
	for s := gChain.AtTheMoney.Prev; s != nil; s = s.Prev {
		if s.Call.Mode == options.ModeNone {
			s.Call.Mode = s.Next.Call.Mode.Invert()
		}
		if s.Put.Mode == options.ModeNone {
			s.Put.Mode = s.Next.Put.Mode.Invert()
		}
	}
}

func onOptionTick(t *databento.CMBP1) *options.Option {
	o := gOptionsByID[t.Header.InstrumentID]
	if o != nil {
		switch t.Action {
		case databento.ActionTrade:
			onOptionTrade(o, t)
		case databento.ActionAdd:
			if t.Header.TSEvent > o.TS {
				onOptionQuote(o, t)
			}
		default:
			panic("unexpected option tick action: " + t.Action.String())
		}
	}
	return o
}

func onOptionTrade(o *options.Option, t *databento.CMBP1) {
}

func onOptionQuote(o *options.Option, t *databento.CMBP1) {
	o.TS = t.Header.TSEvent
	mustRecomputeGreeks := false
	bid := t.Levels[0].BidPx
	if bid != databento.UndefPrice {
		price := decimal.Decimal(bid / 1000)
		if price.Cmp(o.Bid) != 0 {
			mustRecomputeGreeks = true
		}
		o.Bid = price
		o.BidSize = t.Levels[0].BidSz
		o.Got |= options.GotBid
	} else {
		o.Bid = decimal.Zero
		o.BidSize = 0
		o.Got &^= options.GotBid
	}
	ask := t.Levels[0].AskPx
	if ask != databento.UndefPrice {
		price := decimal.Decimal(ask / 1000)
		if price.Cmp(o.Ask) != 0 {
			mustRecomputeGreeks = true
		}
		o.Ask = price
		o.AskSize = t.Levels[0].AskSz
		o.Got |= options.GotAsk
	} else {
		o.Ask = decimal.Zero
		o.AskSize = 0
		o.Got &^= options.GotAsk
	}
	if gChain.Add(o) {
		mustRecomputeGreeks = true
	}
	if mustRecomputeGreeks {
		o.ComputeGreeks(gChain.Price, kRiskFreeRate, decimal.Zero)
	}
}

func compareOrders(a, b *Order) int {
	res := a.Score.Cmp(b.Score)
	if res != 0 {
		return -res // highest score first
	}
	return a.ID - b.ID
}
