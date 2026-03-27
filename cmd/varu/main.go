package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/black76"
	"dropbear/ds/options"
	"dropbear/ds/prob"
	"dropbear/ds/symbol"
	"dropbear/loggy"
	"flag"
	"fmt"
	"log"
	"maps"
	"math"
	"os"
	"os/signal"
	"runtime/pprof"
	"time"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/emirpasic/gods/v2/sets/treeset"
)

var (
	allFlag        = flag.Bool("all", false, "enable all strategies")
	noneFlag       = flag.Bool("none", false, "disable all strategies")
	noliqFlag      = flag.Bool("noliq", false, "disable liquidation strategies")
	comboFlag      = flag.Bool("combo", false, "enable buying and selling combos")
	dbnFlag        = flag.String("dbn", "", "path to backtest data")
	liveFlag       = flag.Bool("live", false, "run in live trading mode")
	webFlag        = flag.Bool("web", false, "enable web dashboard feature")
	rtFlag         = flag.Bool("rt", false, "run backtest in real time mode")
	dryFlag        = flag.Bool("dry", false, "don't send new orders in live mode")
	symbolFlag     = flag.String("symbol", "XSP", "symbol to trade (e.g. XSP, SPXW)")
	dateFlag       = clocky.TimeFlag("date", "2026-03-19", "date of the trades to report")
	sigmasFlag     = decimal.Flag("sigmas", "2.5", "number of sigmas of strikes to consider")
	budgetFlag     = flag.Float64("budget", 5_000, "maximum acceptable loss at current price")
	floorFlag      = flag.Float64("floor", 40_000, "maximum acceptable loss in catastrophic scenario")
	spreadFlag     = decimal.Flag("spread", "-.5", "spread crossing (-1=make, 0=mid, 1=take)")
	pruneFlag      = flag.Float64("prune", .5, "probability of stochastic pruning in strategy search")
	closeFlag      = flag.Float64("closer", .1, "random probability that closing will be allowed")
	demandFlag     = decimal.Flag("demand", "1.1", "minimum ratio of open credit to close cost for liquidation")
	wPayoffFlag    = decimal.Flag("w-payoff", "1", "scoring weight for expected payoff improvement")
	wRiskFlag      = decimal.Flag("w-risk", "1", "scoring weight for risk reduction")
	wDeltaFlag     = decimal.Flag("w-delta", "1", "scoring weight for delta neutrality improvement")
	maxDriftFlag   = decimal.Flag("max-drift", "1", "maximum acceptable accounting drift before panic")
	thinkFlag      = clocky.DurationFlag("think", "1000ms", "interval between backtest trading analysis")
	patienceFlag   = clocky.DurationFlag("patience", "9s", "how long to wait before canceling live orders")
	cooldownFlag   = clocky.DurationFlag("cooldown", "10s", "interval between trading decisions")
	slowdownFlag   = clocky.DurationFlag("slowdown", "200ms", "polling limit for web dashboard")
	heartbeatFlag  = clocky.DurationFlag("heartbeat", "1m", "interval between status reports")
	maxPendingFlag = flag.Int("max-pending", 1, "maximum number of pending orders")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
)

const (
	kXSP         = symbol.Symbol('X' | 'S'<<8 | 'P'<<16)
	kSPXW        = symbol.Symbol('S' | 'P'<<8 | 'X'<<16 | 'W'<<24)
	kStartOfDay  = 9_35_00
	kStopTrading = 14_30_00
	kMarketClose = 16_00_00
	kMultiplier  = 100
)

var (
	gVolume              int
	gCash                decimal.Decimal
	gTotalFees           decimal.Decimal
	gChain               = options.NewOptions()
	gSymbol              symbol.Symbol
	gPositions           = treemap.New[string, decimal.Decimal]()
	gOptionsByID         = map[uint32]*options.Option{}
	gOptionsByOSI        = map[string]*options.Option{}
	gStrategiesUsed      = treemap.New[string, int]()
	gNextTradeTime       clocky.Time
	gBaselinePayoff      decimal.Decimal
	gBaselineWorst       decimal.Decimal
	gBaselineDelta       decimal.Decimal
	gBaselineSettlements []strikeSettlement // precomputed per think cycle
)

// strikeSettlement caches the baseline portfolio settlement value at a
// given strike. Precomputed once per think cycle so that each simulation
// only needs to add the delta from its 2 staged legs, turning O(strikes ×
// positions × simulations) into O(strikes × simulations × 2).
type strikeSettlement struct {
	strike   *options.Strike
	baseline decimal.Decimal // settlement from gPositions only (no cash)
	movement float64         // |price - strike| for risk tolerance
	sigmas   float64         // movement / expectedMove
	blend    float64         // NormCDF blend for risk interpolation
	maxLoss  float64         // interpolated max acceptable loss
}

var (
	gStagedCash        decimal.Decimal
	gStagedPositions   = treemap.New[string, decimal.Decimal]()
	gSimulations       = treeset.NewWith(compareSimulations)
	gSimulationsByID   = map[schwab.OrderID]*Simulation{}
	gSimulationCounter int
	gIdentifierCounter int
	gAbortSimulation   bool
	gEODTransitioned   bool
	gAllowClosing      bool
	gGenerosity        int              // ticks of spread generosity (positive = generous, negative = greedy)
	gFilledOrders      []*Simulation    // filled orders for generosity review
)

var (
	kFeePerContract = decimal.Parse("1.2")
	kRiskFreeRate   = decimal.Parse("0.035")
	kTick01         = decimal.Parse("0.01")
	kTick05         = decimal.Parse("0.05")
	kTick10         = decimal.Parse("0.10")
	kThree          = decimal.FromInt(3)
)

type Simulation struct {
	ID        int
	OrderID   schwab.OrderID
	Legs      *treemap.Map[string, decimal.Decimal]
	Strategy  string
	Price     decimal.Decimal
	Worst     decimal.Decimal
	Payoff    decimal.Decimal // expected payoff at midpoint when order was created
	Score     decimal.Decimal // payoff improvement + risk reduction
	Created   clocky.Time
	Canceling bool
}

func (s *Simulation) String() string {
	return fmt.Sprintf("#%d %s", s.ID, s.Strategy)
}

func main() {
	loggy.Init()
	flag.Parse()
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

func onThink(now clocky.Time) {
	if !beginSimulations(now) {
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
	liquidateCallVertical()
	liquidatePutVertical()
	loggy.Hint("thought for %s (%d simulations)", time.Since(benchStartTime), gSimulationCounter)
	sendBestOrder(now)
}

func beginSimulations(now clocky.Time) bool {
	gSimulationCounter = 0
	precomputeSettlements()
	gBaselinePayoff = computeExpectedPayoff()
	if gBaselinePayoff.Cmp(decimal.NegOne) == 0 {
		loggy.Hint("not thinking: no strikes with positive probability (price=%s em=%s atm=%v)",
			gChain.Price, gChain.ExpectedMove(), gChain.AtTheMoney != nil)
		return false
	}
	gBaselineWorst = computeRisk()
	gBaselineDelta = computeDelta()
	reviewGenerosity()
	if !gEODTransitioned {
		isPowerHour := now.ClockInt() >= kStopTrading
		if isPowerHour {
			*wRiskFlag *= 10
			clear(gStrategyEnabled)
			maps.Copy(gStrategyEnabled, gStrategyEnabledEOD)
			gEODTransitioned = true
			gAllowClosing = true
		}
	}
	return true
}

func sendBestOrder(now clocky.Time) {
	if gSimulations.Empty() {
		return
	}
	loggy.Hint("%d out of %d simulated orders were reasonable",
		gSimulations.Size(), gSimulationCounter)
	it := gSimulations.Iterator()
	it.Next()
	sim := it.Value()
	sim.Created = now
	sim.ID = gIdentifierCounter
	gIdentifierCounter++
	gNextTradeTime = now.Add(*cooldownFlag)
	c, _ := gStrategiesUsed.Get(sim.Strategy)
	gStrategiesUsed.Put(sim.Strategy, c+1)
	if *liveFlag {
		sendLiveOrder(sim)
	} else {
		simulateOrder(sim)
	}
	log.Printf("#%d %s: price=%s score=%s payoff=%s worst=%s",
		sim.ID, sim.Strategy, sim.Price, sim.Score, sim.Payoff.Truncate(), sim.Worst)
	for legIt := sim.Legs.Iterator(); legIt.Next(); {
		sym, pos := legIt.Key(), legIt.Value()
		option := gOptionsByOSI[sym]
		if pos.IsPositive() {
			log.Printf("  buy  %s", option)
		} else {
			log.Printf("  sell %s", option)
		}
	}
	gSimulations.Clear()
}

// buy simulates buying an option. Aborts if we're already short this
// contract, to prevent churning (buying back what we sold).
func buy(option *options.Option) {
	if !gAllowClosing && option.Mode == options.ModeShort {
		gAbortSimulation = true
		return
	}
	gStagedPositions.Put(option.OSI(), decimal.One)
}

// sell simulates selling an option. Aborts if we're already long this
// contract, to prevent churning (selling what we bought).
func sell(option *options.Option) {
	if !gAllowClosing && option.Mode == options.ModeLong {
		gAbortSimulation = true
		return
	}
	gStagedPositions.Put(option.OSI(), decimal.NegOne)
}

// prune returns true if we should skip this branch of the search.
func prune() bool {
	return rando() < *pruneFlag
}

func end(strategy string) bool {
	if gAbortSimulation {
		gAbortSimulation = false
		gStagedPositions.Clear()
		gStagedCash = decimal.Zero
		return false
	}
	// evaluate at midpoint for scoring (spread=0)
	midPrice := chooseMidPrice()
	if !checkSpreadWidth(midPrice) {
		gStagedPositions.Clear()
		gStagedCash = decimal.Zero
		gSimulationCounter++
		return true
	}
	gStagedCash = midPrice.MulInt(kMultiplier)
	if checkRiskTolerance() {
		payoff := computeExpectedPayoff()
		if payoff.Cmp(decimal.NegOne) != 0 {
			orderPrice := choosePriceForSimulation()
			if orderPrice.IsZero() {
				for it := gStagedPositions.Iterator(); it.Next(); {
					opt := gOptionsByOSI[it.Key()]
					loggy.Hint("zero price %s: %s bid=%s ask=%s", strategy, opt, opt.Bid, opt.Ask)
				}
				gStagedPositions.Clear()
				gStagedCash = decimal.Zero
				gSimulationCounter++
				return true
			}
			sim := &Simulation{
				ID:       gSimulationCounter,
				Legs:     gStagedPositions,
				Strategy: strategy,
				Price:    orderPrice,
				Worst:    computeRisk(),
				Payoff:   payoff,
			}
			payoffImprovement := sim.Payoff.Sub(gBaselinePayoff)
			riskReduction := sim.Worst.Sub(gBaselineWorst).Max(decimal.Zero)
			simDelta := computeDelta()
			deltaImprovement := gBaselineDelta.Abs().Sub(simDelta.Abs()).Max(decimal.Zero)
			sim.Score = payoffImprovement.Mul(*wPayoffFlag).Add(riskReduction.Mul(*wRiskFlag)).Add(deltaImprovement.Mul(*wDeltaFlag))
			if sim.Score.IsPositive() {
				gSimulations.Add(sim)
				gStagedPositions = treemap.New[string, decimal.Decimal]()
			}
		}
	}
	gSimulationCounter++
	gStagedCash = decimal.Zero
	gStagedPositions.Clear()
	return true
}

// reviewGenerosity adjusts the generosity level by reviewing all past
// filled orders. For each filled order, compare the current portfolio
// payoff to the payoff at the time the order was created. If things
// improved, the fill was good — add a tick of generosity. If things
// got worse, add a tick of greed. The net effect converges on the
// optimal spread: generous enough to get fills, greedy enough to
// not give away edge.
func reviewGenerosity() {
	if len(gFilledOrders) == 0 {
		return
	}
	gen := 0
	for _, sim := range gFilledOrders {
		if gBaselinePayoff.Cmp(sim.Payoff) > 0 {
			gen++ // payoff improved since this fill — be more generous
		} else {
			gen-- // payoff declined since this fill — be more greedy
		}
	}
	if gen != gGenerosity {
		gGenerosity = gen
		loggy.Hint("generosity=%d (%d filled orders reviewed)", gGenerosity, len(gFilledOrders))
	}
}

// chooseMidPrice computes the net price at midpoint for all staged legs.
// Used for evaluating trade merit independent of spread demands.
func chooseMidPrice() decimal.Decimal {
	price := decimal.Zero
	for it := gStagedPositions.Iterator(); it.Next(); {
		sym, pos := it.Key(), it.Value()
		opt := gOptionsByOSI[sym]
		mid := opt.MarketPrice()
		if pos.IsPositive() {
			price = price.Sub(mid)
		} else {
			price = price.Add(mid)
		}
	}
	return price
}

func choosePriceForSimulation() decimal.Decimal {
	// exec controls where in the spread we price:
	//   -1 = make (bid for buys, ask for sells — most favorable)
	//    0 = mid
	//    1 = take (ask for buys, bid for sells — least favorable)
	// values beyond ±1 go past the spread
	spread := *spreadFlag
	price := decimal.Zero
	slippage := decimal.Parse("0.2")
	for it := gStagedPositions.Iterator(); it.Next(); {
		sym, pos := it.Key(), it.Value()
		opt := gOptionsByOSI[sym]
		mid := opt.MarketPrice()
		hlf := opt.Ask.Sub(opt.Bid).DivInt(2)
		if pos.IsPositive() {
			// buying: mid + halfSpread * spread (round down = cheaper for us)
			legPrice := quantizeTruncate(mid.Add(hlf.Mul(spread)))
			price = price.Sub(legPrice)
		} else {
			// selling: mid - halfSpread * spread (round up = more credit for us)
			legPrice := quantizeAway(mid.Sub(hlf.Mul(spread)))
			price = price.Add(legPrice)
		}
	}
	// apply generosity: positive = we give ticks to the counterparty
	// (reduce credit or increase debit). This makes fills more likely
	// when we're doing well and tightens up when fills go against us.
	for i := 0; i < gGenerosity; i++ {
		price = decTick(price)
	}
	for i := 0; i > gGenerosity; i-- {
		price = incTick(price)
	}
	// clamp: never go more than 20 cents past the quoted spread per
	// leg, to avoid order rejection from runaway generosity/greed.
	midPrice := chooseMidPrice()
	maxSlip := slippage.MulInt(gStagedPositions.Size())
	price = price.Max(midPrice.Sub(maxSlip)).Min(midPrice.Add(maxSlip))
	return price
}

// checkSpreadWidth rejects orders where the net price exceeds the maximum
// possible value of the spread. Schwab will reject these with "Credit
// spreads cannot equal or exceed the strike difference." This happens when
// the robot looks at deep ITM options where individual bid/ask prices
// reflect intrinsic value, making a spread look absurdly profitable. In
// reality no one will fill a $10-wide spread for $18 credit — the spread
// price is bounded by the width between strikes. The same logic applies
// to debits: paying more than the spread width is equally nonsensical.
func checkSpreadWidth(price decimal.Decimal) bool {
	if gStagedPositions.Size() < 2 {
		return true // not a spread
	}
	// find the min and max strike prices among staged legs
	first := true
	var lo, hi decimal.Decimal
	for it := gStagedPositions.Iterator(); it.Next(); {
		opt := gOptionsByOSI[it.Key()]
		sp := opt.Strike.Price
		if first {
			lo, hi = sp, sp
			first = false
		} else {
			lo = lo.Min(sp)
			hi = hi.Max(sp)
		}
	}
	maxValue := hi.Sub(lo)
	return price.Abs().Cmp(maxValue) < 0
}

func onHeartbeat() {
	liq := computeLiquidationValue()
	eod := computeSettlementAt(gChain.Price)
	pay := computeExpectedPayoff()
	bad := computeRisk()
	delta, gamma, theta, vega := computeGreeks()
	log.Printf("cash:%s liq:%s eod:%s worst:%s realized:%s payoff:%s delta:%s gamma:%s theta:%s vega:%s",
		gCash, liq.Truncate(), eod, bad, gRealizedPnL.Truncate(), pay.Truncate(),
		delta.Format(2), gamma.Format(2), theta.Format(2), vega.Format(2))
}

func onEndOfDay() {
	sanityCheck("preSettlement")
	log.Printf("end of day settlement time")
	iteratePositions(func(sym string, pos decimal.Decimal) {
		intrinsic := gOptionsByOSI[sym].IntrinsicValue(gChain.Price)
		settlement := intrinsic.Mul(pos).MulInt(kMultiplier)
		gCash = gCash.Add(settlement)
		log.Printf("have %4s of %s worth %8s", pos, sym, settlement.Truncate())
	})
	log.Printf("strategies used")
	for it := gStrategiesUsed.Iterator(); it.Next(); {
		strategy, count := it.Key(), it.Value()
		log.Printf("%30s %4d", strategy, count)
	}
	log.Printf("ending balance %s less %s fees winning %s at %s price %s",
		gCash.Format(2), gTotalFees.Format(2), gCash.Sub(gTotalFees).Format(2), gSymbol, gChain.Price)
}

// iteratePositions calls f for all positions.
func iteratePositions(f func(string, decimal.Decimal)) {
	iteratePositionsImpl(gPositions, f)
	iteratePositionsImpl(gStagedPositions, f)
}

func iteratePositionsImpl(positions *treemap.Map[string, decimal.Decimal], f func(string, decimal.Decimal)) {
	for it := positions.Iterator(); it.Next(); {
		sym, pos := it.Key(), it.Value()
		if !pos.IsZero() {
			f(sym, pos)
		}
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
// so that simulations only need to add the delta from their 2 staged legs.
func precomputeSettlements() {
	em := gChain.ExpectedMove().Float64()
	gBaselineSettlements = gBaselineSettlements[:0]
	for it := gChain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		value := decimal.Zero
		for pit := gPositions.Iterator(); pit.Next(); {
			sym, pos := pit.Key(), pit.Value()
			if !pos.IsZero() {
				intrinsic := gOptionsByOSI[sym].IntrinsicValue(strike.Price)
				value = value.Add(intrinsic.Mul(pos))
			}
		}
		ss := strikeSettlement{
			strike:   strike,
			baseline: value.MulInt(kMultiplier),
		}
		if em > 0 {
			ss.movement = gChain.Price.Sub(strike.Price).Abs().Float64()
			ss.sigmas = ss.movement / em
			ss.blend = (prob.NormCDF(ss.sigmas) - .5) * 2
			ss.maxLoss = math.FMA(*floorFlag-*budgetFlag, ss.blend, *budgetFlag)
		}
		gBaselineSettlements = append(gBaselineSettlements, ss)
	}
}

// stagedSettlementDelta computes the incremental settlement contribution
// from only the staged positions at the given underlying price.
func stagedSettlementDelta(underlyingPrice decimal.Decimal) decimal.Decimal {
	value := decimal.Zero
	for sit := gStagedPositions.Iterator(); sit.Next(); {
		sym, pos := sit.Key(), sit.Value()
		if !pos.IsZero() {
			intrinsic := gOptionsByOSI[sym].IntrinsicValue(underlyingPrice)
			value = value.Add(intrinsic.Mul(pos))
		}
	}
	return value.MulInt(kMultiplier)
}

// computeRisk calculates the worst possible outcome of the current portfolio.
func computeRisk() decimal.Decimal {
	worst := decimal.Zero
	for _, ss := range gBaselineSettlements {
		settlement := ss.baseline.Add(gCash)
		if gStagedPositions.Size() > 0 {
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
		settlement := ss.baseline.Add(gCash)
		if gStagedPositions.Size() > 0 {
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
	iteratePositions(func(sym string, pos decimal.Decimal) {
		intrinsic := gOptionsByOSI[sym].IntrinsicValue(underlyingPrice)
		value = value.Add(intrinsic.Mul(pos))
	})
	return value.MulInt(kMultiplier).Add(gCash).Add(gStagedCash)
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

// computePayoffSlow is the non-cached version for the dashboard.
func computePayoffSlow() decimal.Decimal {
	type strikeProb struct {
		strike *options.Strike
		prob   decimal.Decimal
	}
	var probs []strikeProb
	var probSum decimal.Decimal
	iterateStrikes(func(strike *options.Strike) {
		p := strike.Probability()
		if p.IsPositive() {
			probs = append(probs, strikeProb{strike, p})
			probSum = probSum.Add(p)
		}
	})
	if len(probs) == 0 || !probSum.IsPositive() {
		return decimal.NegOne
	}
	payoff := decimal.Zero
	for _, sp := range probs {
		p := sp.prob.Div(probSum)
		iteratePositions(func(sym string, pos decimal.Decimal) {
			intrinsic := gOptionsByOSI[sym].IntrinsicValue(sp.strike.Price)
			payoff = payoff.Add(intrinsic.Mul(pos).Mul(p))
		})
	}
	return payoff.MulInt(kMultiplier).Add(gCash).Add(gStagedCash)
}

func computeLiquidationValue() decimal.Decimal {
	value := decimal.Zero
	iteratePositions(func(sym string, pos decimal.Decimal) {
		mid := gOptionsByOSI[sym].MarketPrice()
		value = value.Add(mid.Mul(pos))
	})
	return value.MulInt(kMultiplier).Add(gCash).Add(gStagedCash)
}

// computeNotional returns the total underlying notional value controlled
// by all positions: sum(abs(qty)) * underlying_price * 100.
func computeNotional() decimal.Decimal {
	var totalQty decimal.Decimal
	iteratePositions(func(sym string, pos decimal.Decimal) {
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
		return decimal.NegOne
	}
	// compute expected payoff using cached baselines
	payoff := decimal.Zero
	for _, cp := range probs {
		ss := &gBaselineSettlements[cp.idx]
		settlement := ss.baseline
		if gStagedPositions.Size() > 0 {
			settlement = settlement.Add(stagedSettlementDelta(ss.strike.Price))
		}
		payoff = payoff.Add(settlement.Mul(cp.prob.Div(probSum)))
	}
	return payoff.Add(gCash).Add(gStagedCash)
}

// computeBias returns what direction options market thinks underlying will move.
func computeBias() decimal.Decimal {
	atm := gChain.AtTheMoney
	if atm == nil || !atm.IsReady() {
		return decimal.Zero
	}
	return atm.Call.MarketPrice().Sub(atm.Put.MarketPrice()).MulInt(100)
}

// computeDelta calculates the delta for all positions.
func computeDelta() decimal.Decimal {
	delta := decimal.Zero
	iteratePositions(func(sym string, pos decimal.Decimal) {
		delta = delta.Add(gOptionsByOSI[sym].Delta.Mul(pos))
	})
	return delta.MulInt(kMultiplier)
}

func computeGreeks() (delta, gamma, theta, vega decimal.Decimal) {
	iteratePositions(func(sym string, pos decimal.Decimal) {
		delta = delta.Add(gOptionsByOSI[sym].Delta.Mul(pos))
		gamma = gamma.Add(gOptionsByOSI[sym].Gamma.Mul(pos))
		theta = theta.Add(gOptionsByOSI[sym].Theta.Mul(pos))
		vega = vega.Add(gOptionsByOSI[sym].Vega.Mul(pos))
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
	for it := gPositions.Iterator(); it.Next(); {
		sym, pos := it.Key(), it.Value()
		option := gOptionsByOSI[sym]
		if pos.IsPositive() {
			option.Mode = options.ModeLong
		} else if pos.IsNegative() {
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

func onOptionTick(t *databento.CMBP1) {
	if o := gOptionsByID[t.Header.InstrumentID]; o != nil {
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
	} else {
		loggy.Hint("tick for unknown instrument id %d", t.Header.InstrumentID)
	}
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

func compareSimulations(a, b *Simulation) int {
	res := a.Score.Cmp(b.Score)
	if res != 0 {
		return -res // highest score first
	}
	return a.ID - b.ID
}
