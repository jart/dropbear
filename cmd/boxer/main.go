package main

import (
	"flag"
	"io"
	"log"
	"strings"
	"time"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
)

var (
	symbol        = flag.String("symbol", "SPXW", "underlying symbol")
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
	records          int
	lastDefTS        clocky.Time
	bestObservedEdge decimal.Decimal
	liveDB           *defBuilder
	pairsByInst      map[uint32][]*boxPair
	instruments      map[uint32]optionInfo
	quotes           map[uint32]*quote
	pairs            []boxPair
	book             *ledger
	brk              *broker
	active           []*activeBox
	results          []boxResult
	completedCount   int
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()
	clocky.Now = clocky.FakeNow

	bestObservedEdge = decimal.FromInt(-999)
	pairsByInst = make(map[uint32][]*boxPair)
	quotes = make(map[uint32]*quote)
	book = &ledger{position: make(map[uint32]int), cash: *cashFlag}
	brk = &broker{rate: 100, limit: 100}
	active = make([]*activeBox, 0)
	results = make([]boxResult, 0)
	liveDB = newDefBuilder()

	client, err := databento.Dial(*dataset, databento.MustLoadDefaultKey())
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()

	client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeParent,
		Symbols: []string{*symbol + ".OPT"},
		Start:   clocky.MustParseTime("2026-03-02T06:30:05"),
	})

	client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaCMBP1S,
		SType:   databento.STypeParent,
		Symbols: []string{*symbol + ".OPT"},
		Start:   clocky.MustParseTime("2026-03-02T06:30:05"),
	})

	meta, err := client.Start()
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	log.Printf("streaming %s (dbn v%d)", *symbol+".OPT", meta.Version)

	for {
		rec, err := client.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("decode: %v", err)
		}
		records++
		switch m := rec.(type) {
		case *databento.ErrorMsg:
			log.Printf("gateway error: %s", m.Err)
			return
		case *databento.SystemMsg:
			log.Printf("system message: %s", m.Msg)
			if strings.Contains(m.Msg, "End of interval for") {
				return
			}
		case *databento.SymbolMappingMsg:
			log.Printf("symbol mapping: %s (%s) -> %s (%s)", m.GetSTypeInSymbol(), m.STypeIn, m.GetSTypeOutSymbol(), m.STypeOut)
			id := m.Header.InstrumentID
			osi := m.GetSTypeOutSymbol()
			date := time.Now()
			queryDateInt := date.Year()*10000 + int(date.Month())*100 + date.Day()
			if id != 0 && osi != "" && liveDB.addInstrumentFromOSI(id, osi, queryDateInt) {
				instruments = liveDB.instruments
				pairs = liveDB.buildPairs(*widthFlag)
				pairsByInst = make(map[uint32][]*boxPair)
				for i := range pairs {
					p := &pairs[i]
					for _, pid := range []uint32{p.callLoID, p.callHiID, p.putLoID, p.putHiID} {
						pairsByInst[pid] = append(pairsByInst[pid], p)
					}
				}
			}
		case *databento.CBBO:
			onTick(m.TSRecv, m.Header.InstrumentID, dbnPrice(m.Levels[0].BidPx), dbnPrice(m.Levels[0].AskPx))
		case *databento.CMBP1:
			onTick(m.TSRecv, m.Header.InstrumentID, dbnPrice(m.Levels[0].BidPx), dbnPrice(m.Levels[0].AskPx))
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
}

func onTick(ts clocky.Time, instID uint32, bid, ask decimal.Decimal) {
	info, ok := instruments[instID]
	if !ok {
		return
	}
	q, ok := quotes[instID]
	if !ok {
		q = &quote{}
		quotes[instID] = q
	}
	q.bid = bid
	q.ask = ask
	clocky.SetNow(ts)
	log.Printf("hello")

	for _, ab := range active {
		for i, ord := range ab.orders {
			if ord == nil || ord.instID != instID {
				continue
			}
			if q.bid.IsZero() || q.ask.IsZero() || q.ask.Cmp(q.bid) <= 0 {
				continue
			}
			if !ord.pendingPx.IsZero() && clocky.Since(ord.modifyTS) >= *latencyFlag {
				ord.limitPx, ord.postTime, ord.pendingPx = ord.pendingPx, ord.modifyTS, decimal.Zero
			}
			if clocky.Since(ord.postTime) < *latencyFlag {
				continue
			}
			mid := q.bid.Add(q.ask).DivInt(2)
			var crossed bool
			var fillPx decimal.Decimal
			if ord.side == 'B' {
				if ord.limitPx.Cmp(mid) >= 0 {
					crossed, fillPx = true, ord.limitPx.Min(snapAsk(q.ask))
				}
			} else {
				if ord.limitPx.Cmp(mid) <= 0 {
					crossed, fillPx = true, ord.limitPx.Max(snapBid(q.bid))
				}
			}
			if crossed {
				ab.legs[i] = &boxLeg{instID: instID, class: info.class, strike: info.strike, side: ord.side, fillPx: fillPx, fillTS: ts, orderTS: ord.postTime}
				ab.orders[i] = nil
				ab.filled++
				book.recordFill(ts, instID, ord.side, fillPx)
			} else {
				greed := computeGreed(*greedFlag, clocky.Duration(ts-ab.startTS), *patienceFlag, ab.filled)
				newPx := greedyMid(q, ord.side, greed)
				budget := budgetLimit(ab, i, *widthFlag, quotes)
				newPx = clampLimit(ord.side, newPx, budget)
				if !newPx.IsZero() && !newPx.IsNegative() && newPx.Cmp(ord.limitPx) != 0 {
					brk.modifyOrder(ord, newPx, ts)
				}
			}
		}
	}

	j := 0
	for _, ab := range active {
		if ab.filled == 4 {
			var br boxResult
			var netOutflow decimal.Decimal
			br.pair, br.long, br.filled, br.startTS = ab.pair, ab.long, 4, ab.startTS
			for i, leg := range ab.legs {
				br.legs[i] = *leg
				if leg.side == 'B' {
					netOutflow = netOutflow.Add(leg.fillPx)
				} else {
					netOutflow = netOutflow.Sub(leg.fillPx)
				}
			}
			if ab.long {
				br.pnl = (*widthFlag).Sub(netOutflow)
			} else {
				br.pnl = netOutflow.Neg().Sub(*widthFlag)
			}
			results, completedCount = append(results, br), completedCount+1
			continue
		}
		active[j] = ab
		j++
	}
	active = active[:j]

	if len(active) < *maxOpen && brk.canCall(ts, 4) {
		var bestPair *boxPair
		var bestLong bool
		var bestEdge, bestDebit decimal.Decimal
		for i := range pairs {
			bp := &pairs[i]
			if hasActivePair(active, bp) {
				continue
			}
			qCL, qCH, qPL, qPH := quotes[bp.callLoID], quotes[bp.callHiID], quotes[bp.putLoID], quotes[bp.putHiID]
			if qCL == nil || qCH == nil || qPL == nil || qPH == nil {
				continue
			}
			if !legQualified(qCL) || !legQualified(qCH) || !legQualified(qPL) || !legQualified(qPH) {
				continue
			}
			lD := snapBid(qCL.bid).Add(snapBid(qPH.bid)).Sub(snapAsk(qCH.ask)).Sub(snapAsk(qPL.ask))
			lE := (*widthFlag).Sub(lD)
			sC := snapAsk(qCL.ask).Add(snapAsk(qPH.ask)).Sub(snapBid(qCH.bid)).Sub(snapBid(qPL.bid))
			sE := sC.Sub(*widthFlag)
			var edge, debit decimal.Decimal
			var long bool
			if lE.Cmp(sE) >= 0 {
				edge, long, debit = lE, true, lD
			} else {
				edge, long, debit = sE, false, sC.Neg()
			}
			if (edge.Cmp(*edgeFlag) >= 0 && edge.Cmp(*maxEdgeFlag) <= 0) && (bestPair == nil || edge.Cmp(bestEdge) > 0) {
				bestPair, bestLong, bestEdge, bestDebit = bp, long, edge, debit
			} else if *verbose && edge.Cmp(*edgeFlag) >= 0 {
				log.Printf("DEBUG rejected edge=%s (min=%s max=%s) %d/%d", formatPnl(edge), formatPnl(*edgeFlag), formatPnl(*maxEdgeFlag), bp.loStrike.Int(), bp.hiStrike.Int())
			}
		}
		if bestPair != nil {
			ab := startBox(brk, bestPair, bestLong, quotes, ts, *widthFlag)
			active = append(active, ab)
			if *verbose {
				log.Printf("  box #%d %d/%d  edge: %s", len(results)+len(active), bestPair.loStrike.Int(), bestPair.hiStrike.Int(), formatPnl(bestEdge))
			}
			_ = bestDebit
		}
	}
}
