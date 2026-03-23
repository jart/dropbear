package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/black76"
	"dropbear/ds/options"
	"dropbear/ds/symbol"
	"dropbear/loggy"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/emirpasic/gods/v2/sets/treeset"
)

var (
	dateFlag       = clocky.TimeFlag("date", "2026-03-19", "date of the trades to report")
	quotesFlag     = flag.String("quotes", "", "path to DBN file containing SPX quotes")
	defsFlag       = flag.String("defs", "", "path to DBN file containing SPX definitions")
	sigmasFlag     = decimal.Flag("sigmas", "2.5", "number of sigmas of strikes to consider")
	riskFlag       = decimal.Flag("risk", "15_000", "maximum risk allowed")
	execFlag       = flag.String("exec", "mid", "order execution strategy (mid, take, or make)")
	thinkFlag      = clocky.DurationFlag("think", "250ms", "interval between trading analysis")
	cooldownFlag   = clocky.DurationFlag("cooldown", "10s", "interval between trading decisions")
	heartbeatFlag  = clocky.DurationFlag("heartbeat", "1m", "interval between status reports")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
)

const (
	kSPXW       = symbol.Symbol('S' | 'P'<<8 | 'X'<<16 | 'W'<<24)
	kStartOfDay = 9_35_00
	kEndOfDay   = 16_00_00
)

var (
	gSPX               = options.NewOptions()
	gCash              decimal.Decimal
	gPositions         = treemap.New[string, decimal.Decimal]()
	gStagedCash        decimal.Decimal
	gStagedPositions   = treemap.New[string, decimal.Decimal]()
	gOptionsByID       = map[uint32]*options.Option{}
	gOptionsByOSI      = map[string]*options.Option{}
	gPendingStrikes    = treemap.New[decimal.Decimal, *options.Strike]()
	gSimulations       = treeset.NewWith(compareSimulations)
	gSimulationCounter int
	gNextTradeTime     clocky.Time
	gAbortSimulations  bool
	gBaselinePayoff    decimal.Decimal
	gBaselineWorst     decimal.Decimal
)

var (
	kRiskFreeRate = decimal.Parse("0.035")
	kMultiplier   = decimal.FromInt(100)
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
}

func main() {
	loggy.Init()
	flag.Parse()
	black76.DaysPerYear = 252
	black76.HoursPerDay = 6.5
	loadDefinitions(*defsFlag)
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
	quoteReader, err := databento.OpenFileReader(*quotesFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", *quotesFlag, err)
		os.Exit(1)
	}
	defer quoteReader.Close()
	if quoteReader.Metadata.Schema != databento.SchemaCMBP1 {
		fmt.Fprintf(os.Stderr, "%s: expected quotes DBN file with schema %d, got %d\n",
			*quotesFlag, databento.SchemaCMBP1, quoteReader.Metadata.Schema)
		os.Exit(1)
	}
	clocky.Now = clocky.FakeNow
	clocky.Sleep = clocky.FakeSleep
	var nextThought, nextHeartbeat clocky.Time
	for {
		rec, err := quoteReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "%s: read quote error: %v\n", *quotesFlag, err)
			os.Exit(1)
		}
		m := rec.(*databento.CMBP1)
		now := m.TSRecv
		clocky.SetNow(now)
		onOptionTick(m)
		clock := now.ClockInt()
		if clock >= kEndOfDay {
			break
		}
		if clock < kStartOfDay {
			continue
		}
		if now >= nextThought && now >= gNextTradeTime {
			nextThought = now.Add(*thinkFlag)
			onThink()
		}
		if now >= nextHeartbeat {
			nextHeartbeat = now.Add(*heartbeatFlag)
			onHeartbeat()
		}
	}
	onEndOfDay()
}

func onThink() {
	gSimulationCounter = 0
	gBaselinePayoff = computeExpectedPayoff()
	gBaselineWorst = computeRisk()
	em := gSPX.ExpectedMove().Mul(*sigmasFlag)
	lo := gSPX.Price.Sub(em)
	hi := gSPX.Price.Add(em)
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
	gCash = gCash.Add(sim.Price.Mul(kMultiplier))
	gNextTradeTime = clocky.Now().Add(*cooldownFlag)
	for legIt := sim.Legs.Iterator(); legIt.Next(); {
		sym, pos := legIt.Key(), legIt.Value()
		existing, _ := gPositions.Get(sym)
		gPositions.Put(sym, existing.Add(pos))
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
	for strike := gSPX.AtTheMoney; strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		buy(strike.Call)
		endSimulation("buy call")
	}
}

func simulateBuyPuts(lo decimal.Decimal) {
	for strike := gSPX.AtTheMoney; strike != nil && strike.Price.Cmp(lo) >= 0; strike = strike.Prev {
		buy(strike.Put)
		endSimulation("buy put")
	}
}

func simulateSellCallVerticals(hi decimal.Decimal) {
	for strike := gSPX.AtTheMoney.Next; strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		sell(gSPX.AtTheMoney.Call)
		buy(strike.Call)
		endSimulation("sell call vertical")
	}
}

func simulateSellPutVerticals(lo decimal.Decimal) {
	for strike := gSPX.AtTheMoney.Prev; strike != nil && strike.Price.Cmp(lo) >= 0; strike = strike.Prev {
		sell(gSPX.AtTheMoney.Put)
		buy(strike.Put)
		endSimulation("sell put vertical")
	}
}

func simulateBuyCombo(lo, hi decimal.Decimal) {
	for _, strike, _ := gSPX.Strikes.Floor(lo); strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		buy(strike.Call)
		sell(strike.Put)
		if endSimulation("buy combo") {
			break
		}
	}
}

func simulateSellCombo(lo, hi decimal.Decimal) {
	for _, strike, _ := gSPX.Strikes.Floor(lo); strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
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
	return true
	// pos1, _ := gPositions.Get(option.OSI())
	// pos2, _ := gStagedPositions.Get(option.OSI())
	// return pos1.Cmp(decimal.Zero) >= 0 && pos2.Cmp(decimal.Zero) >= 0
}

func canSell(option *options.Option) bool {
	return true
	// pos1, _ := gPositions.Get(option.OSI())
	// pos2, _ := gStagedPositions.Get(option.OSI())
	// return pos1.Cmp(decimal.Zero) <= 0 && pos2.Cmp(decimal.Zero) <= 0
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
	simulation.Score = payoffImprovement.Add(riskReduction.DivInt(10))
	if simulation.Worst.Cmp(gBaselinePayoff.Sub(*riskFlag)) >= 0 {
		gSimulations.Add(simulation)
	}
	gStagedPositions = treemap.New[string, decimal.Decimal]()
	gStagedCash = decimal.Zero
	return true
}

func choosePriceForSimulation(simulation *Simulation) {
	var price decimal.Decimal
	for it := simulation.Legs.Iterator(); it.Next(); {
		sym, pos := it.Key(), it.Value()
		o := gOptionsByOSI[sym]
		switch *execFlag {
		case "mid":
			mid := o.MarketPrice()
			if pos.IsPositive() {
				price = price.Sub(mid)
			} else {
				price = price.Add(mid)
			}
		case "take":
			if pos.IsPositive() {
				price = price.Sub(decTickSPX(o.Ask))
			} else {
				price = price.Add(incTickSPX(o.Bid))
			}
		case "make":
			if pos.IsPositive() {
				price = price.Sub(o.Bid)
			} else {
				price = price.Add(o.Ask)
			}
		default:
			panic("invalid exec strategy: " + *execFlag)
		}
	}
	simulation.Price = price
}

func onHeartbeat() {
	liq := computeLiquidationValue()
	eod := computeSettlementAt(gSPX.Price)
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
		intrinsic := gOptionsByOSI[sym].IntrinsicValue(gSPX.Price)
		settlement := intrinsic.Mul(pos).Mul(kMultiplier)
		gCash = gCash.Add(settlement)
		log.Printf("have %4s of %s worth %8s", pos, sym, settlement.Truncate())
	})
	log.Printf("ending balance %s at spx price %s", gCash.Format(2), gSPX.Price)
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
	em := gSPX.ExpectedMove().Mul(*sigmasFlag)
	lo := gSPX.Price.Sub(em)
	hi := gSPX.Price.Add(em)
	_, strike, _ := gSPX.Strikes.Floor(lo)
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
	for it := gSPX.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		settlement := computeSettlementAt(strike.Price)
		worst = worst.Min(settlement)
	}
	return worst
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
			// log.Printf("warning: strike %s has bad probability %s (spx=%s | call got=%s bid=%s/%d ask=%s/%d | put got=%s bid=%s/%d ask=%s/%d)\n",
			// 	strike.Price, prob, gSPX.Price,
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

func computeGreeks() (delta, gamma, theta, vega decimal.Decimal) {
	iteratePositions(func(sym string, pos decimal.Decimal) {
		delta = delta.Add(gOptionsByOSI[sym].Delta.Mul(pos).Mul(kMultiplier))
		gamma = gamma.Add(gOptionsByOSI[sym].Gamma.Mul(pos).Mul(kMultiplier))
		theta = theta.Add(gOptionsByOSI[sym].Theta.Mul(pos).Mul(kMultiplier))
		vega = vega.Add(gOptionsByOSI[sym].Vega.Mul(pos))
	})
	return delta, gamma, theta, vega
}

func loadSchwabOrder(order *schwab.Order) {
	if order.Status != schwab.OrderStatusFilled {
		return
	}
	for _, leg := range order.OrderLegCollection {
		if leg.Instrument.AssetType != schwab.AssetTypeOption {
			continue
		}
		option := gOptionsByOSI[leg.Instrument.Symbol]
		if option == nil {
			continue
		}
		for _, activity := range order.OrderActivityCollection {
			if activity.ExecutionType != schwab.ExecutionTypeFill {
				continue
			}
			for _, execLeg := range activity.ExecutionLegs {
				if execLeg.InstrumentID != leg.Instrument.InstrumentID {
					continue
				}
				if execLeg.LegID != leg.LegID {
					continue
				}
				price := execLeg.Price
				switch leg.Instruction {
				case schwab.InstructionBuyToOpen, schwab.InstructionBuyToClose:
					price = price.Neg()
				}
				for range execLeg.Quantity.Int() {
					cost := price.Mul(kMultiplier)
					gCash = gCash.Add(cost)
					sym := option.OSI()
					pos, _ := gPositions.Get(sym)
					if cost.IsNegative() {
						pos = pos.Add(decimal.One) // bought
					} else {
						pos = pos.Sub(decimal.One) // sold
					}
					gPositions.Put(sym, pos)
				}
			}
		}
	}
}

func loadDefinitions(path string) {
	defReader, err := databento.OpenFileReader(path)
	if err != nil {
		fmt.Printf("%s: %v\n", path, err)
		os.Exit(1)
	}
	defer defReader.Close()
	if defReader.Metadata.Schema != databento.SchemaDefinition {
		fmt.Printf("%s: expected definitions DBN file with schema %d, got %d\n", path, databento.SchemaDefinition, defReader.Metadata.Schema)
		os.Exit(1)
	}
	wantYear, wantMonth, wantDay := dateFlag.Date()
	for {
		rec, err := defReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("%s: read error: %v\n", path, err)
			os.Exit(1)
		}
		switch m := rec.(type) {
		case *databento.Instrument:
			sym := symbol.MustParse(m.GetAsset())
			year, timeMonth, day := m.Expiration.In(clocky.UTC).Date()
			month := clocky.Month(timeMonth)
			if sym == kSPXW && year == wantYear && month == wantMonth && day == wantDay {
				rawSymbol := m.GetRawSymbol()
				id := m.Header.InstrumentID
				strike := decimal.Decimal(m.StrikePrice / 1000)
				class := m.InstrumentClass
				option := &options.Option{
					ID:     id,
					Class:  class,
					Strike: &options.Strike{Price: strike},
					Sym:    sym,
					Year:   year,
					Month:  month,
					Day:    day,
				}
				gOptionsByID[id] = option
				gOptionsByOSI[rawSymbol] = option
			}
		}
	}
	if len(gOptionsByID) == 0 {
		fmt.Fprintf(os.Stderr, "no spxw 0dte definitions found\n")
		os.Exit(1)
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
	if gSPX.Add(o) {
		mustRecomputeGreeks = true
	}
	if mustRecomputeGreeks {
		o.ComputeGreeks(gSPX.Price, kRiskFreeRate, decimal.Zero)
	}
}

// incTickSPX increases an spx option's price by one tick.
func incTickSPX(price decimal.Decimal) decimal.Decimal {
	if price.Abs().Cmp(kThree) <= 0 {
		return price.Add(kTick05)
	}
	return price.Add(kTick10)
}

// decTickSPX reduces an spx option's price by one tick.
func decTickSPX(price decimal.Decimal) decimal.Decimal {
	if price.Abs().Cmp(kThree) <= 0 {
		return price.Sub(kTick05)
	}
	return price.Sub(kTick10)
}

func compareSimulations(a, b *Simulation) int {
	res := a.Score.Cmp(b.Score)
	if res != 0 {
		return -res // highest score first
	}
	return a.Sequence - b.Sequence
}
