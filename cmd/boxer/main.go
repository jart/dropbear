package main

import (
	"flag"
	"io"
	"log"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
)

var (
	symbolFlag    = flag.String("symbol", "SPXW", "underlying symbol")
	dataset       = flag.String("dataset", "OPRA.PILLAR", "dataset")
	dataPath      = flag.String("data", "", "path to data")
	defsPath      = flag.String("defs", "", "path to defs")
	widthFlag     = decimal.Flag("width", "50", "box width")
	tickFlag      = decimal.Flag("tick", "0.05", "tick")
	multFlag      = flag.Int("mult", 100, "multiplier")
	edgeFlag      = decimal.Flag("edge", "0.10", "min edge")
	maxEdgeFlag   = decimal.Flag("maxedge", "10.00", "max edge")
	maxSpreadFlag = decimal.Flag("maxspread", "1.00", "max spread")
	minBidFlag    = decimal.Flag("minbid", "0.05", "min bid")
	maxOpen       = flag.Int("maxopen", 4, "max open")
	latencyFlag   = clocky.DurationFlag("latency", "70ms", "latency")
	greedFlag     = decimal.Flag("greed", "0.00", "greed factor for spread placement")
	patienceFlag  = clocky.DurationFlag("patience", "500ms", "patience")
	cashFlag      = decimal.Flag("cash", "100000", "starting cash")
	verbose       = flag.Bool("v", false, "verbose")
)

var (
	cash        decimal.Decimal
	optionsByID = make(map[uint32]*Options) // stores all possible strikes for today's 0dte option chain, keyed by instrument id
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()
	cash = *cashFlag
	liveTrade()
}

func liveTrade() {
	client, err := databento.Dial(*dataset, databento.MustLoadDefaultKey())
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()

	client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeParent,
		Symbols: []string{*symbolFlag + ".OPT"},
	})

	client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaTCBBO,
		SType:   databento.STypeParent,
		Symbols: []string{*symbolFlag + ".OPT"},
	})

	meta, err := client.Start()
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	log.Printf("streaming %s (dbn v%d)", *symbolFlag+".OPT", meta.Version)

	for {
		rec, err := client.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("decode: %v", err)
		}
		switch m := rec.(type) {
		case *databento.ErrorMsg:
			log.Printf("gateway error: %s", m.Err)
			return
		case *databento.SystemMsg:
			log.Printf("system message: %s", m.Msg)
		case *databento.SymbolMappingMsg:
			onSymbolMapping(m.Header.InstrumentID, m.GetSTypeOutSymbol())
		case *databento.CBBO:
			onTick(m.TSRecv, m.Header.InstrumentID, dbnPrice(m.Levels[0].BidPx), dbnPrice(m.Levels[0].AskPx))
		case *databento.CMBP1:
			onTick(m.TSRecv, m.Header.InstrumentID, dbnPrice(m.Levels[0].BidPx), dbnPrice(m.Levels[0].AskPx))
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
}

func onSymbolMapping(id uint32, str string) {
	log.Printf("map %d -> %s", id, str)
	sym, strike, class, year, month, day, err := parseOSI(str)
	if err != nil {
		log.Printf("failed to parse OSI: %v", err)
		return
	}
	now := clocky.Now().In(clocky.UTC)
	todayYear, todayMonth, todayDay := now.Date()
	if year != todayYear || month != todayMonth || day != todayDay {
		return
	}
	optionsByID[id] = &Options{
		ID:     id,
		Class:  class,
		Sym:    sym,
		Strike: strike,
		Year:   year,
		Month:  month,
		Day:    day,
	}
}

func onTick(ts clocky.Time, instID uint32, bid, ask decimal.Decimal) {
	ops := optionsByID[instID]
	if ops == nil {
		return
	}
	ops.Bid = bid
	ops.Ask = ask
	log.Printf("tick %s bid %6s ask %6s", ops, bid, ask)
}
