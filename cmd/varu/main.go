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
	dbnFlag        = flag.String("dbn", "", "path to backtest data")
	liveFlag       = flag.Bool("live", false, "run in live trading mode")
	webFlag        = flag.Bool("web", false, "enable web dashboard feature")
	rtFlag         = flag.Bool("rt", false, "run backtest in real time mode")
	dryFlag        = flag.Bool("dry", false, "don't send new orders in live mode")
	colorFlag      = flag.Bool("color", false, "enable option chain strike coloring")
	symbolFlag     = flag.String("symbol", "XSP", "symbol to trade (e.g. XSP, SPXW)")
	dateFlag       = clocky.TimeFlag("date", "2026-03-19", "date of the trades to report")
	sigmasFlag     = decimal.Flag("sigmas", "2", "number of sigmas of strikes to consider")
	budgetFlag     = flag.Float64("budget", 5_000, "maximum acceptable loss at current price")
	floorFlag      = flag.Float64("floor", 40_000, "maximum acceptable loss in catastrophic scenario")
	spreadFlag     = decimal.Flag("spread", "-.5", "spread crossing (-1=make, 0=mid, 1=take)")
	pruneFlag      = flag.Float64("prune", .5, "probability of stochastic pruning in strategy search")
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
	gCash           decimal.Decimal
	gChain          = options.NewOptions()
	gSymbol         symbol.Symbol
	gPositions      = treemap.New[string, decimal.Decimal]()
	gOptionsByID    = map[uint32]*options.Option{}
	gOptionsByOSI   = map[string]*options.Option{}
	gStrategiesUsed = treemap.New[string, int]()
	gNextTradeTime  clocky.Time
	gBaselinePayoff decimal.Decimal
	gBaselineWorst  decimal.Decimal
	gBaselineDelta  decimal.Decimal
)

var (
	gVolume            int
	gRiskFloor         float64
	gRiskBudget        float64
	gTotalFees         decimal.Decimal
	gStagedCash        decimal.Decimal
	gStagedPositions   = treemap.New[string, decimal.Decimal]()
	gSimulations       = treeset.NewWith(compareSimulations)
	gSimulationsByID   = map[schwab.OrderID]*Simulation{}
	gSimulationCounter int
	gIdentifierCounter int
	gAbortSimulation   bool
	gEODTransitioned   bool
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
	Payoff    decimal.Decimal
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
	em_near := gChain.ExpectedMove()
	lo_near := gChain.Price.Sub(em_near)
	hi_near := gChain.Price.Add(em_near)
	em_wide := gChain.ExpectedMove().Mul(*sigmasFlag)
	lo_wide := gChain.Price.Sub(em_wide)
	hi_wide := gChain.Price.Add(em_wide)
	buyPut()
	buyCall()
	buyCombo(lo_near, hi_near)
	sellCombo(lo_near, hi_near)
	sellCallVertical(lo_wide, hi_wide)
	sellPutVertical(lo_wide, hi_wide)
	buyCallVertical(lo_near, hi_wide)
	buyPutVertical(lo_wide, hi_near)
	liquidatePut()
	liquidateCall()
	liquidateCallVertical()
	liquidatePutVertical()
	log.Printf("thought for %s (%d simulations)", time.Since(benchStartTime), gSimulationCounter)
	sendBestOrder(now)
}

func beginSimulations(now clocky.Time) bool {
	gSimulationCounter = 0
	gBaselinePayoff = computeExpectedPayoff()
	if gBaselinePayoff.Cmp(decimal.NegOne) == 0 {
		return false
	}
	gBaselineWorst = computeRisk()
	gBaselineDelta = computeDelta()
	if !gEODTransitioned {
		isPowerHour := now.ClockInt() >= kStopTrading
		if isPowerHour {
			clear(gStrategyEnabled)
			maps.Copy(gStrategyEnabled, gStrategyEnabledEOD)
			gEODTransitioned = true
		}
	}
	return true
}

func sendBestOrder(now clocky.Time) {
	if gSimulations.Empty() {
		return
	}
	log.Printf("%d out of %d simulated orders were reasonable",
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

// buy simulates buying an option.
func buy(option *options.Option) {
	if canBuy(option) {
		buyWithTheForce(option)
	} else {
		gAbortSimulation = true
	}
}

// sell simulates selling an option.
func sell(option *options.Option) {
	if canSell(option) {
		sellWithTheForce(option)
	} else {
		gAbortSimulation = true
	}
}

// prune returns true if we should skip this branch of the search.
func prune() bool {
	return rando() < *pruneFlag
}

func buyWithTheForce(option *options.Option) {
	gStagedPositions.Put(option.OSI(), decimal.One)
}

func sellWithTheForce(option *options.Option) {
	gStagedPositions.Put(option.OSI(), decimal.NegOne)
}

func canBuy(option *options.Option) bool {
	if option.Mode == options.ModeSellOnly {
		return false
	}
	return true
}

func canSell(option *options.Option) bool {
	if option.Mode == options.ModeBuyOnly {
		return false
	}
	return true
}

func end(strategy string) bool {
	if gAbortSimulation {
		gAbortSimulation = false
		gStagedPositions.Clear()
		gStagedCash = decimal.Zero
		return false
	}
	price := choosePriceForSimulation()
	gStagedCash = price.MulInt(kMultiplier)
	if checkRiskTolerance() {
		payoff := computeExpectedPayoff()
		if payoff.Cmp(decimal.NegOne) != 0 {
			sim := &Simulation{
				ID:       gSimulationCounter,
				Legs:     gStagedPositions,
				Strategy: strategy,
				Price:    price,
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

func choosePriceForSimulation() decimal.Decimal {
	// exec controls where in the spread we price:
	//   -1 = make (bid for buys, ask for sells — most favorable)
	//    0 = mid
	//    1 = take (ask for buys, bid for sells — least favorable)
	// values beyond ±1 go past the spread
	spread := *spreadFlag
	price := decimal.Zero
	for it := gStagedPositions.Iterator(); it.Next(); {
		sym, pos := it.Key(), it.Value()
		opt := gOptionsByOSI[sym]
		mid := opt.MarketPrice()
		hlf := opt.Ask.Sub(opt.Bid).DivInt(2)
		if pos.IsPositive() {
			// buying: mid + halfSpread * spread (round down = cheaper for us)
			price = price.Sub(quantizeTruncate(mid.Add(hlf.Mul(spread))))
		} else {
			// selling: mid - halfSpread * spread (round up = more credit for us)
			price = price.Add(quantizeAway(mid.Sub(hlf.Mul(spread))))
		}
	}
	return price
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
	log.Printf("ending balance %s at %s price %s", gCash.Format(2), gSymbol, gChain.Price)
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

// computeRisk calculates the worst possible outcome of the current portfolio.
func computeRisk() decimal.Decimal {
	worst := decimal.Zero
	for it := gChain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		settlement := computeSettlementAt(strike.Price)
		worst = worst.Min(settlement)
	}
	return worst
}

// checkRiskTolerance verifies that the simulated position's loss at every
// strike is within the acceptable risk tolerance for that probability level.
// Uses the normal CDF to smoothly interpolate between the everyday budget
// (at current price) and the catastrophic floor (at extreme moves).
func checkRiskTolerance() bool {
	em := gChain.ExpectedMove().Float64()
	if em <= 0 {
		return false
	}
	for it := gChain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		settlement := computeSettlementAt(strike.Price).Float64()
		movement := gChain.Price.Sub(strike.Price).Abs().Float64()
		sigmas := movement / em
		// NormCDF(0)=0.5, NormCDF(3)≈0.999. We want 0σ→budget, ∞σ→floor.
		// Scale so: 0σ→0, ∞σ→1
		blend := (prob.NormCDF(sigmas) - .5) * 2                        // 0 at 0σ, ~1 at 3+σ
		maxLoss := math.FMA(gRiskFloor-gRiskBudget, blend, gRiskBudget) // lerp budget→floor
		if settlement < -maxLoss {
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
	// collect probabilities and sum them
	type strikeProb struct {
		strike *options.Strike
		prob   decimal.Decimal
	}
	var probs []strikeProb
	var probSum decimal.Decimal
	iterateStrikes(func(strike *options.Strike) {
		prob := strike.Probability()
		if prob.IsPositive() {
			probs = append(probs, strikeProb{strike, prob})
			probSum = probSum.Add(prob)
		}
	})
	if len(probs) == 0 || !probSum.IsPositive() {
		return decimal.NegOne
	}
	// compute expected payoff with normalized probabilities
	payoff := decimal.Zero
	for _, sp := range probs {
		prob := sp.prob.Div(probSum)
		iteratePositions(func(sym string, pos decimal.Decimal) {
			intrinsic := gOptionsByOSI[sym].IntrinsicValue(sp.strike.Price)
			payoff = payoff.Add(intrinsic.Mul(pos).Mul(prob))
		})
	}
	return payoff.MulInt(kMultiplier).Add(gCash).Add(gStagedCash)
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
	if *colorFlag {
		colorStrikes()
	}
}

func colorStrikes() {
	mode := options.ModeBuyOnly
	for it := gChain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		strike.Call.Mode = mode
		strike.Put.Mode = mode.Flip()
		mode = mode.Flip()
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
