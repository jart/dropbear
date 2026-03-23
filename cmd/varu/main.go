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
	"log"
	"os"
	"os/signal"
	"runtime/pprof"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/emirpasic/gods/v2/sets/treeset"
)

var (
	symbolFlag     = flag.String("symbol", "XSP", "symbol to trade (e.g. XSP, SPXW)")
	dateFlag       = clocky.TimeFlag("date", "2026-03-19", "date of the trades to report")
	sigmasFlag     = decimal.Flag("sigmas", "2.5", "number of sigmas of strikes to consider")
	budgetFlag     = decimal.Flag("budget", "5_000", "maximum acceptable loss at current price")
	floorFlag      = decimal.Flag("floor", "50_000", "maximum acceptable loss in catastrophic scenario")
	execFlag       = decimal.Flag("exec", "-.5", "spread crossing (-1=make, 0=mid, 1=take)")
	cooldownFlag   = clocky.DurationFlag("cooldown", "10s", "interval between trading decisions")
	patienceFlag   = clocky.DurationFlag("patience", "9s", "how long to wait before canceling live orders")
	heartbeatFlag  = clocky.DurationFlag("heartbeat", "1m", "interval between status reports")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
)

const (
	kXSP        = symbol.Symbol('X' | 'S'<<8 | 'P'<<16)
	kSPXW       = symbol.Symbol('S' | 'P'<<8 | 'X'<<16 | 'W'<<24)
	kStartOfDay = 9_35_00
	kEndOfDay   = 16_00_00
)

var (
	gCash           decimal.Decimal
	gChain          = options.NewOptions()
	gSymbol         symbol.Symbol
	gPositions      = treemap.New[string, decimal.Decimal]()
	gOptionsByID    = map[uint32]*options.Option{}
	gOptionsByOSI   = map[string]*options.Option{}
	gNextTradeTime  clocky.Time
	gBaselinePayoff decimal.Decimal
	gBaselineWorst  decimal.Decimal
	gBaselineDelta  decimal.Decimal
)

var (
	gStagedCash        decimal.Decimal
	gStagedPositions   = treemap.New[string, decimal.Decimal]()
	gSimulations       = treeset.NewWith(compareSimulations)
	gSimulationsByID   = map[schwab.OrderID]*Simulation{}
	gSimulationCounter int
	gAbortSimulations  bool
)

var (
	kRiskFreeRate = decimal.Parse("0.035")
	kMultiplier   = decimal.FromInt(100)
	kTick01       = decimal.Parse("0.01")
	kTick05       = decimal.Parse("0.05")
	kTick10       = decimal.Parse("0.10")
	kThree        = decimal.FromInt(3)
)

type Simulation struct {
	Legs     *treemap.Map[string, decimal.Decimal]
	Strategy string
	Price    decimal.Decimal
	Worst    decimal.Decimal
	Payoff   decimal.Decimal
	Score    decimal.Decimal // payoff improvement + risk reduction
	Sequence int
	Created  clocky.Time
	OrderID  schwab.OrderID
}

func (s *Simulation) String() string {
	var legs []string
	for it := s.Legs.Iterator(); it.Next(); {
		sym, pos := it.Key(), it.Value()
		action := "buy"
		if pos.IsNegative() {
			action = "sell"
		}
		legs = append(legs, action+" "+sym)
	}
	return s.Strategy + " " + s.Price.Format(2) + " " + s.Payoff.Format(2) + " " + s.Worst.Format(2) + " [" + stringJoin(legs, ", ") + "]"
}

func main() {
	loggy.Init()
	flag.Parse()
	black76.DaysPerYear = 252
	black76.HoursPerDay = 6.5
	gSymbol = symbol.MustParse(*symbolFlag)
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
	if *backtestFlag {
		backtest()
	} else {
		live()
	}
}

func onThink() {
	gSimulationCounter = 0
	gBaselinePayoff = computeExpectedPayoff()
	gBaselineWorst = computeRisk()
	gBaselineDelta = computeDelta()
	em := gChain.ExpectedMove().Mul(*sigmasFlag)
	lo := gChain.Price.Sub(em)
	hi := gChain.Price.Add(em)
	simulateBuyCalls(hi)
	simulateBuyPuts(lo)
	simulateSellCallVerticals(hi)
	simulateSellPutVerticals(lo)
	simulateBuyCombo(lo, hi)
	simulateSellCombo(lo, hi)
	if gSimulations.Empty() {
		return
	}
	it := gSimulations.Iterator()
	it.Next()
	sim := it.Value()
	if !sim.Score.IsPositive() {
		gSimulations.Clear()
		return
	}
	now := clocky.Now()
	sim.Created = now
	gNextTradeTime = now.Add(*cooldownFlag)
	if *backtestFlag {
		gCash = gCash.Add(sim.Price.Mul(kMultiplier))
		for legIt := sim.Legs.Iterator(); legIt.Next(); {
			sym, pos := legIt.Key(), legIt.Value()
			existing, _ := gPositions.Get(sym)
			gPositions.Put(sym, existing.Add(pos))
		}
	} else {
		sendLiveOrder(sim)
	}
	log.Printf("%s: price=%s payoff=%s worst=%s", sim.Strategy, sim.Price, sim.Payoff.Truncate(), sim.Worst)
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

func simulateBuyCalls(hi decimal.Decimal) {
	for strike := gChain.AtTheMoney; strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		buy(strike.Call)
		endSimulation("buy call")
	}
}

func simulateBuyPuts(lo decimal.Decimal) {
	for strike := gChain.AtTheMoney; strike != nil && strike.Price.Cmp(lo) >= 0; strike = strike.Prev {
		buy(strike.Put)
		endSimulation("buy put")
	}
}

func simulateSellCallVerticals(hi decimal.Decimal) {
	for strike := gChain.AtTheMoney.Next; strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		sell(gChain.AtTheMoney.Call)
		buy(strike.Call)
		endSimulation("sell call vertical")
	}
}

func simulateSellPutVerticals(lo decimal.Decimal) {
	for strike := gChain.AtTheMoney.Prev; strike != nil && strike.Price.Cmp(lo) >= 0; strike = strike.Prev {
		sell(gChain.AtTheMoney.Put)
		buy(strike.Put)
		endSimulation("sell put vertical")
	}
}

func simulateBuyCombo(lo, hi decimal.Decimal) {
	for _, strike, _ := gChain.Strikes.Floor(lo); strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		buy(strike.Call)
		sell(strike.Put)
		if endSimulation("buy combo") {
			break
		}
	}
}

func simulateSellCombo(lo, hi decimal.Decimal) {
	for _, strike, _ := gChain.Strikes.Floor(lo); strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		sell(strike.Call)
		buy(strike.Put)
		if endSimulation("sell combo") {
			break
		}
	}
}

func buy(option *options.Option) {
	if canBuy(option) {
		gStagedPositions.Put(option.OSI(), decimal.One)
	} else {
		gAbortSimulations = true
	}
}

func sell(option *options.Option) {
	if canSell(option) {
		gStagedPositions.Put(option.OSI(), decimal.NegOne)
	} else {
		gAbortSimulations = true
	}
}

func canBuy(option *options.Option) bool {
	pos1, _ := gPositions.Get(option.OSI())
	pos2, _ := gStagedPositions.Get(option.OSI())
	return pos1.Cmp(decimal.Zero) >= 0 && pos2.Cmp(decimal.Zero) >= 0
}

func canSell(option *options.Option) bool {
	pos1, _ := gPositions.Get(option.OSI())
	pos2, _ := gStagedPositions.Get(option.OSI())
	return pos1.Cmp(decimal.Zero) <= 0 && pos2.Cmp(decimal.Zero) <= 0
}

func endSimulation(strategy string) bool {
	if gAbortSimulations {
		gAbortSimulations = false
		gStagedPositions.Clear()
		gStagedCash = decimal.Zero
		return false
	}
	simulation := &Simulation{Legs: gStagedPositions, Strategy: strategy}
	simulation.Sequence = gSimulationCounter
	gSimulationCounter++
	choosePriceForSimulation(simulation)
	gStagedCash = simulation.Price.Mul(kMultiplier)
	simulation.Worst = computeRisk()
	simulation.Payoff = computeExpectedPayoff()
	payoffImprovement := simulation.Payoff.Sub(gBaselinePayoff)
	riskReduction := simulation.Worst.Sub(gBaselineWorst).Max(decimal.Zero)
	simDelta := computeDelta()
	deltaImprovement := gBaselineDelta.Abs().Sub(simDelta.Abs()).Max(decimal.Zero)
	simulation.Score = payoffImprovement.Add(riskReduction.DivInt(10)).Add(deltaImprovement)
	if checkRiskTolerance() {
		gSimulations.Add(simulation)
	}
	gStagedPositions = treemap.New[string, decimal.Decimal]()
	gStagedCash = decimal.Zero
	return true
}

func choosePriceForSimulation(simulation *Simulation) {
	// exec controls where in the spread we price:
	//   -1 = make (bid for buys, ask for sells — most favorable)
	//    0 = mid
	//    1 = take (ask for buys, bid for sells — least favorable)
	// values beyond ±1 go past the spread
	var price decimal.Decimal
	exec := *execFlag
	for it := simulation.Legs.Iterator(); it.Next(); {
		sym, pos := it.Key(), it.Value()
		o := gOptionsByOSI[sym]
		mid := o.MarketPrice()
		halfSpread := o.Ask.Sub(o.Bid).DivInt(2)
		if pos.IsPositive() {
			// buying: mid + halfSpread * exec (round down = cheaper for us)
			price = price.Sub(quantizeTruncate(mid.Add(halfSpread.Mul(exec))))
		} else {
			// selling: mid - halfSpread * exec (round up = more credit for us)
			price = price.Add(quantizeAway(mid.Sub(halfSpread.Mul(exec))))
		}
	}
	simulation.Price = price
}

func onHeartbeat() {
	liq := computeLiquidationValue()
	eod := computeSettlementAt(gChain.Price)
	pay := computeExpectedPayoff()
	worst := computeRisk()
	delta, gamma, theta, vega := computeGreeks()
	log.Printf("cash:%s liq:%s eod:%s worst:%s payoff:%s delta:%s gamma:%s theta:%s vega:%s",
		gCash, liq.Truncate(), eod, worst, pay.Truncate(),
		delta.Format(2), gamma.Format(2), theta.Format(2), vega.Format(2))
}

func onEndOfDay() {
	log.Printf("end of day settlement time")
	iteratePositions(func(sym string, pos decimal.Decimal) {
		intrinsic := gOptionsByOSI[sym].IntrinsicValue(gChain.Price)
		settlement := intrinsic.Mul(pos).Mul(kMultiplier)
		gCash = gCash.Add(settlement)
		log.Printf("have %4s of %s worth %8s", pos, sym, settlement.Truncate())
	})
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
	em := gChain.ExpectedMove()
	if !em.IsPositive() {
		return false
	}
	budget := *budgetFlag
	floor := *floorFlag
	for it := gChain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		settlement := computeSettlementAt(strike.Price)
		sigmas := gChain.Price.Sub(strike.Price).Abs().Div(em).Float64()
		// NormCDF(0)=0.5, NormCDF(3)≈0.999. We want 0σ→budget, ∞σ→floor.
		// Scale so: 0σ→0, ∞σ→1
		blend := (prob.NormCDF(sigmas) - 0.5) * 2    // 0 at 0σ, ~1 at 3+σ
		maxLoss := budget.Add(floor.Sub(budget).Mul( // lerp budget→floor
			decimal.FromFloat64(blend)))
		if settlement.Cmp(maxLoss.Neg()) < 0 {
			return false
		}
	}
	return true
}

func computeSettlementAt(underlyingPrice decimal.Decimal) decimal.Decimal {
	cash := gCash + gStagedCash
	iteratePositions(func(sym string, pos decimal.Decimal) {
		intrinsic := gOptionsByOSI[sym].IntrinsicValue(underlyingPrice)
		settlement := intrinsic.Mul(pos).Mul(kMultiplier)
		cash = cash.Add(settlement)
	})
	return cash
}

func computeLiquidationValue() decimal.Decimal {
	cash := gCash + gStagedCash
	iteratePositions(func(sym string, pos decimal.Decimal) {
		mid := gOptionsByOSI[sym].MarketPrice()
		value := mid.Mul(pos).Mul(kMultiplier)
		cash = cash.Add(value)
	})
	return cash
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
		} else {
			// log.Printf("warning: strike %s has bad probability %s (%s=%s | call got=%s bid=%s/%d ask=%s/%d | put got=%s bid=%s/%d ask=%s/%d)\n",
			// 	strike.Price, prob, gSymbol, gChain.Price,
			// 	strike.Call.Got,
			// 	strike.Call.Bid, strike.Call.BidSize,
			// 	strike.Call.Ask, strike.Call.AskSize,
			// 	strike.Put.Got,
			// 	strike.Put.Bid, strike.Put.BidSize,
			// 	strike.Put.Ask, strike.Put.AskSize)
		}
	})
	if len(probs) == 0 || !probSum.IsPositive() {
		panic("unexpectedly found no strikes with positive probability")
	}
	// compute expected payoff with normalized probabilities
	payoff := gCash + gStagedCash
	for _, sp := range probs {
		prob := sp.prob.Div(probSum)
		iteratePositions(func(sym string, pos decimal.Decimal) {
			intrinsic := gOptionsByOSI[sym].IntrinsicValue(sp.strike.Price)
			payoff = payoff.Add(intrinsic.Mul(pos).Mul(kMultiplier).Mul(prob))
		})
	}
	return payoff
}

func computeDelta() decimal.Decimal {
	var delta decimal.Decimal
	iteratePositions(func(sym string, pos decimal.Decimal) {
		delta = delta.Add(gOptionsByOSI[sym].Delta.Mul(pos).Mul(kMultiplier))
	})
	return delta
}

func computeGreeks() (delta, gamma, theta, vega decimal.Decimal) {
	iteratePositions(func(sym string, pos decimal.Decimal) {
		delta = delta.Add(gOptionsByOSI[sym].Delta.Mul(pos).Mul(kMultiplier))
		gamma = gamma.Add(gOptionsByOSI[sym].Gamma.Mul(pos).Mul(kMultiplier))
		theta = theta.Add(gOptionsByOSI[sym].Theta.Mul(pos).Mul(kMultiplier))
		vega = vega.Add(gOptionsByOSI[sym].Vega.Mul(pos))
	})
	return delta, gamma, theta, vega
}

func onOptionDef(o *options.Option) {
	gOptionsByID[o.ID] = o
	gOptionsByOSI[o.OSI()] = o
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
	return a.Sequence - b.Sequence
}
