// replay is a tool for replaying historical 0dte SPXW option trades using historical quotes from a DBN file, e.g.
// day=03-19; go run ./cmd/replay -date 2026-${day} -orders ~/scratch/schwab-orders-2026-${day}.json -defs ~/databento/SPXW/2026-${day}.definition.dbn -quotes ~/databento/SPXW/2026-${day}.cmbp-1.0dte.dbn | tee ~/scratch/boxreport-2026-${day}.txt
package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/options"
	"dropbear/ds/symbol"
	"dropbear/loggy"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

var (
	dateFlag       = clocky.TimeFlag("date", "2026-03-19", "date of the trades to report")
	ordersFlag     = flag.String("orders", "", "path to JSON file containing Schwab orders")
	quotesFlag     = flag.String("quotes", "", "path to DBN file containing SPX quotes")
	defsFlag       = flag.String("defs", "", "path to DBN file containing SPX definitions")
	rangeFlag      = decimal.Flag("range", "200", "possible movement of spx for risk calculation")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
)

const (
	kSPXW = symbol.Symbol('S' | 'P'<<8 | 'X'<<16 | 'W'<<24)
)

var (
	gCash           decimal.Decimal
	gSPX            = options.NewOptions()
	gPositions      = map[string]int{}
	gOptionsByID    = map[uint32]*options.Option{}
	gOptionsByOSI   = map[string]*options.Option{}
	gTrades         = binaryheap.NewWith(compareTradeByTime)
	gPendingStrikes = treemap.New[decimal.Decimal, *options.Strike]()
	kRiskFreeRate   = decimal.Parse("0.035")
	kMultiplier     = decimal.FromInt(100)
	kTick           = decimal.FromInt(5)
)

type Trade struct {
	Option *options.Option
	Cost   decimal.Decimal
	Time   clocky.Time
	Tag    string
}

func compareTradeByTime(a, b *Trade) int {
	if a.Time.Before(b.Time) {
		return -1
	} else if a.Time.After(b.Time) {
		return 1
	}
	return 0
}

func main() {
	loggy.Init()
	flag.Parse()

	// open business data
	loadDefinitions(*defsFlag)
	loadSchwabOrders(*ordersFlag)

	if *flagCPUProfile != "" {
		f, err := os.Create(*flagCPUProfile)
		if err != nil {
			log.Fatalf("could not create CPU profile: %v", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatalf("could not start CPU profile: %v", err)
		}
		defer pprof.StopCPUProfile()
	}

	// open quotes file (this is big)
	quoteReader, err := databento.OpenFileReader(*quotesFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", *quotesFlag, err)
		os.Exit(1)
	}
	defer quoteReader.Close()
	if quoteReader.Metadata.Schema != databento.SchemaCMBP1 {
		fmt.Fprintf(os.Stderr, "%s: expected quotes DBN file with schema %d, got %d\n", *quotesFlag, databento.SchemaCMBP1, quoteReader.Metadata.Schema)
		os.Exit(1)
	}

	// consume historical quotes while replaying our trades
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
		if m.TSRecv.Hour() >= 16 {
			break
		}
		onOptionTick(m)
		trade, ok := gTrades.Peek()
		if !ok || trade.Time.After(m.TSRecv) {
			continue
		}
		gTrades.Pop()
		cash := trade.Cost
		gCash = gCash.Add(cash)
		if trade.Cost.IsNegative() {
			gPositions[trade.Option.OSI()]++ // bought
		} else {
			gPositions[trade.Option.OSI()]-- // sold
		}
		liq := computeLiquidationValue()
		eod := computeSettlementAt(gSPX.Price)
		pay := computeExpectedPayoff()
		best, worst := computeRisk()
		fmt.Printf("%02d:%02d:%02d %s have %+3d of %s %c cost: %6s cash: %8s liq: %8s eod: %6s best: %6s worst: %6s payoff: %6s\n",
			trade.Time.Hour(), trade.Time.Minute(), trade.Time.Second(), describeTag(trade.Tag),
			gPositions[trade.Option.OSI()], trade.Option.Strike, trade.Option.Class,
			trade.Cost, gCash, liq.Format(2), eod, best, worst, pay.Format(2))
	}

	// settle remaining positions at close
	fmt.Printf("\nend of day settlement:\n")
	for contract, pos := range gPositions {
		if pos == 0 {
			continue
		}
		intrinsic := gOptionsByOSI[contract].IntrinsicValue(gSPX.Price)
		settlement := intrinsic.MulInt(pos).Mul(kMultiplier)
		gCash = gCash.Add(settlement)
		fmt.Printf("have %3d of %s worth %10s\n", pos, contract, settlement.Format(2))
	}
	fmt.Printf("\nending balance %s at spx price %s\n", gCash.Format(2), gSPX.Price)
}

func computeRisk() (best, worst decimal.Decimal) {
	lo := gSPX.Price.Sub(*rangeFlag)
	hi := gSPX.Price.Add(*rangeFlag)
	_, strike, _ := gSPX.Strikes.Ceiling(lo)
	for ; strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		settlement := computeSettlementAt(strike.Price)
		worst = worst.Min(settlement)
		best = best.Max(settlement)
	}
	return best, worst
}

func computeSettlementAt(underlyingPrice decimal.Decimal) decimal.Decimal {
	cash := gCash
	for sym, pos := range gPositions {
		intrinsic := gOptionsByOSI[sym].IntrinsicValue(underlyingPrice)
		settlement := intrinsic.MulInt(pos).Mul(kMultiplier)
		cash = cash.Add(settlement)
	}
	return cash
}

func computeLiquidationValue() decimal.Decimal {
	cash := gCash
	for sym, pos := range gPositions {
		mid := gOptionsByOSI[sym].MarketPrice()
		value := mid.MulInt(pos).Mul(kMultiplier)
		cash = cash.Add(value)
	}
	return cash
}

func computeExpectedPayoff() decimal.Decimal {
	payoff := decimal.Zero
	lo := gSPX.Price.Sub(*rangeFlag)
	hi := gSPX.Price.Add(*rangeFlag)
	_, strike, _ := gSPX.Strikes.Ceiling(lo)
	for ; strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		settlement := computeSettlementAt(strike.Price)
		payoff = payoff.Add(settlement.Mul(strike.Probability()))
	}
	return payoff
}

func getTrades(path string, order *schwab.Order) {
	if order.Status != schwab.OrderStatusFilled {
		log.Printf("%s: warning: skipping order %d with status %s\n", path, order.OrderID, order.Status)
		return
	}
	for _, leg := range order.OrderLegCollection {
		if leg.Instrument.AssetType != schwab.AssetTypeOption {
			log.Printf("%s: warning: skipping leg with non-option asset type %s\n", path, leg.Instrument.AssetType)
			continue
		}
		option := gOptionsByOSI[leg.Instrument.Symbol]
		if option == nil {
			log.Printf("%s: warning: no option definition found for leg with symbol %s\n", path, leg.Instrument.Symbol)
			continue
		}
		for _, activity := range order.OrderActivityCollection {
			if activity.ExecutionType != schwab.ExecutionTypeFill {
				log.Printf("%s: warning: skipping activity with non-fill execution type %s\n", path, activity.ExecutionType)
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
					gTrades.Push(&Trade{
						Option: option,
						Cost:   price.Mul(kMultiplier),
						Time:   execLeg.Time,
						Tag:    order.Tag,
					})
				}
			}
		}
	}
}

func loadSchwabOrders(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
		os.Exit(1)
	}
	var batch []schwab.Order
	if err := json.Unmarshal(data, &batch); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
		os.Exit(1)
	}
	for _, order := range batch {
		getTrades(path, &order)
	}
	if gTrades.Empty() {
		fmt.Fprintf(os.Stderr, "no filled 0dte spxw option trades found\n")
		os.Exit(1)
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

func describeTag(tag string) string {
	if strings.HasPrefix(tag, "TA_") {
		return "API" // schwab api
	}
	if strings.Contains(tag, "TOS") {
		return "TOS" // thinkorswim
	}
	return tag
}
