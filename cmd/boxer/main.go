package main

import (
	"bufio"
	"flag"
	"io"
	"log"
	"time"
	"unsafe"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
)

var (
	records          int
	lastDefTS        clocky.Time
	bestObservedEdge decimal.Decimal
)

var (
	symbol        = flag.String("symbol", "SPXW", "underlying symbol")
	dataset       = flag.String("dataset", "OPRA.PILLAR", "dataset")
	dataPath      = flag.String("data", "", "path to data")
	defsPath      = flag.String("defs", "", "path to defs")
	widthInt      = flag.Int("width", 50, "box width")
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
	cashInt       = flag.Int("cash", 100000, "starting cash")
	verbose       = flag.Bool("v", false, "verbose")
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()

	width := decimal.FromInt(*widthInt)
	bestObservedEdge = decimal.FromInt(-999)

	date := time.Now()
	queryDateInt := date.Year()*10000 + int(date.Month())*100 + date.Day()

	var startMTV, cutoffMTV, expiryMTV clocky.Time

	var instruments map[uint32]optionInfo
	var pairs []boxPair
	pairsByInst := make(map[uint32][]*boxPair)
	var liveDB *defBuilder

	client, err := databento.Dial(*dataset, databento.MustLoadDefaultKey())
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()
	liveDB = newDefBuilder()
	parentSym := *symbol + ".OPT"
	log.Printf("subscribing to %s on %s...", parentSym, *dataset)
	err = client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeParent,
		Symbols: []string{parentSym},
	})
	if err != nil {
		log.Fatalf("subscribe definitions: %v", err)
	}
	err = client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaCMBP1,
		SType:   databento.STypeParent,
		Symbols: []string{parentSym},
	})
	if err != nil {
		log.Fatalf("subscribe: %v", err)
	}
	meta, err := client.Start()
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	log.Printf("streaming %s (dbn v%d)...", parentSym, meta.Version)

	log.Printf("Starting main processing loop...")
	records = 0
	cmbp1Size := int(unsafe.Sizeof(databento.CMBP1{}))
	quotes := make(map[uint32]*quote)
	book := &ledger{position: make(map[uint32]int), cash: decimal.FromInt(*cashInt)}
	brk := &broker{rate: 100, limit: 100}
	active := make([]*activeBox, 0)
	var results []boxResult
	var completedCount int
	var doneTrading bool

	for {
		rec, err := client.NextRecord()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("decode: %v", err)
		}
		records++

		if len(rec) >= 2 {
			rtype := databento.RType(rec[1])
			switch rtype {
			case databento.RTypeInstrumentDef:
				continue // live uses SymbolMapping IDs; backtest uses loadDefinitions
			case databento.RTypeSymbolMapping:
				if liveDB != nil {
					if m, ok := databento.CastRecord(rec).(*databento.SymbolMappingMsg); ok {
						id := m.Header.InstrumentID
						osi := m.GetSTypeOutSymbol()
						if id != 0 && osi != "" && liveDB.addInstrumentFromOSI(id, osi, queryDateInt) {
							instruments = liveDB.instruments
							pairs = liveDB.buildPairs(*widthInt)
							pairsByInst = make(map[uint32][]*boxPair)
							for i := range pairs {
								p := &pairs[i]
								for _, pid := range []uint32{p.callLoID, p.callHiID, p.putLoID, p.putHiID} {
									pairsByInst[pid] = append(pairsByInst[pid], p)
								}
							}
						}
					}
				}
				continue
			case databento.RTypeCMBP1:
				break
			case databento.RTypeError:
				msg := databento.CastRecord(rec).(databento.ErrorMsg)
				log.Printf("gateway error: %s", msg.Err)
				continue
			case databento.RTypeSystem:
				continue
			}
		}

		if len(rec) < cmbp1Size {
			continue
		}
		cmbp1Tick := (*databento.CMBP1)(unsafe.Pointer(&rec[0]))

		if records%100000 == 0 || clocky.Since(lastDefTS) > 10*clocky.Second {
			var quotedPairs, qualifiedPairs int
			if !doneTrading {
				for i := range pairs {
					bp := &pairs[i]
					qCL, qCH, qPL, qPH := quotes[bp.callLoID], quotes[bp.callHiID], quotes[bp.putLoID], quotes[bp.putHiID]
					if qCL == nil || qCH == nil || qPL == nil || qPH == nil {
						continue
					}
					quotedPairs++
					lD := snapBid(qCL.bidPx).Add(snapBid(qPH.bidPx)).Sub(snapAsk(qCH.askPx)).Sub(snapAsk(qPL.askPx))
					lE := width.Sub(lD)
					sC := snapAsk(qCL.askPx).Add(snapAsk(qPH.askPx)).Sub(snapBid(qCH.bidPx)).Sub(snapBid(qPL.bidPx))
					sE := sC.Sub(width)
					edge := lE
					if sE.Cmp(lE) > 0 {
						edge = sE
					}
					if bestObservedEdge.Cmp(decimal.FromInt(-900)) < 0 || edge.Cmp(bestObservedEdge) > 0 {
						bestObservedEdge = edge
					}
					if legQualified(qCL) && legQualified(qCH) && legQualified(qPL) && legQualified(qPH) {
						qualifiedPairs++
					}
				}
			}
			log.Printf("live: recs=%d insts=%d pairs=%d quotes=%d quoted=%d qual=%d best_edge=%s done=%v", records, len(instruments), len(pairs), len(quotes), quotedPairs, qualifiedPairs, formatPnl(bestObservedEdge), doneTrading)
			bestObservedEdge, lastDefTS = decimal.FromInt(-999), clocky.Now()
		}

		instID := cmbp1Tick.Header.InstrumentID
		info, ok := instruments[instID]
		if !ok {
			continue
		}
		ts := cmbp1Tick.Header.TSEvent

		q, ok := quotes[instID]
		if !ok {
			q = &quote{}
			quotes[instID] = q
		}
		bidPx, askPx := cmbp1Tick.Levels[0].BidPx, cmbp1Tick.Levels[0].AskPx
		if bidPx != databento.UndefPrice {
			q.bidPx, q.bidSz = dbnPrice(bidPx), cmbp1Tick.Levels[0].BidSz
		} else {
			q.bidPx = decimal.Zero
		}
		if askPx != databento.UndefPrice {
			q.askPx, q.askSz = dbnPrice(askPx), cmbp1Tick.Levels[0].AskSz
		} else {
			q.askPx = decimal.Zero
		}

		if ts.Before(startMTV) {
			continue
		}

		for _, ab := range active {
			for i, ord := range ab.orders {
				if ord == nil || ord.instID != instID {
					continue
				}
				if q.bidPx.IsZero() || q.askPx.IsZero() || q.askPx.Cmp(q.bidPx) <= 0 {
					continue
				}
				if !ord.pendingPx.IsZero() && clocky.Since(ord.modifyTS) >= *latencyFlag {
					ord.limitPx, ord.postTime, ord.pendingPx = ord.pendingPx, ord.modifyTS, decimal.Zero
				}
				if clocky.Since(ord.postTime) < *latencyFlag {
					continue
				}
				mid := q.bidPx.Add(q.askPx).DivInt(2)
				var crossed bool
				var fillPx decimal.Decimal
				if ord.side == 'B' {
					if ord.limitPx.Cmp(mid) >= 0 {
						crossed, fillPx = true, ord.limitPx.Min(snapAsk(q.askPx))
					}
				} else {
					if ord.limitPx.Cmp(mid) <= 0 {
						crossed, fillPx = true, ord.limitPx.Max(snapBid(q.bidPx))
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
					budget := budgetLimit(ab, i, width, quotes)
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
				var netOutflow decimal.Decimal
				var br boxResult
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
					br.pnl = width.Sub(netOutflow)
				} else {
					br.pnl = netOutflow.Neg().Sub(width)
				}
				results, completedCount = append(results, br), completedCount+1
				continue
			}
			active[j] = ab
			j++
		}
		active = active[:j]

		if expiryMTV != 0 && !ts.Before(expiryMTV) {
			break
		}
		if cutoffMTV != 0 && !ts.Before(cutoffMTV) {
			if len(active) == 0 {
				doneTrading = true
			}
			continue
		}

		if len(active) < *maxOpen && brk.canCall(ts, 4) && !doneTrading {
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
				lD := snapBid(qCL.bidPx).Add(snapBid(qPH.bidPx)).Sub(snapAsk(qCH.askPx)).Sub(snapAsk(qPL.askPx))
				lE := width.Sub(lD)
				sC := snapAsk(qCL.askPx).Add(snapAsk(qPH.askPx)).Sub(snapBid(qCH.bidPx)).Sub(snapBid(qPL.bidPx))
				sE := sC.Sub(width)
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
				ab := startBox(brk, bestPair, bestLong, quotes, ts, width)
				active = append(active, ab)
				if *verbose {
					log.Printf("  box #%d %d/%d  edge: %s", len(results)+len(active), bestPair.loStrike.Int(), bestPair.hiStrike.Int(), formatPnl(bestEdge))
				}
				_ = bestDebit
			}
		}
	}
	printSummary(book, results, completedCount, width)
}

func loadDefinitions(r io.Reader, widthInt int, queryDateInt int) (map[uint32]optionInfo, []boxPair, decimal.Decimal, error) {
	db := newDefBuilder()
	br := bufio.NewReader(r)
	for {
		rec, err := databento.DecodeRecord(br)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, decimal.Zero, err
		}
		if len(rec) >= int(unsafe.Sizeof(databento.Instrument{})) {
			db.addInstrument((*databento.Instrument)(unsafe.Pointer(&rec[0])), queryDateInt)
		}
	}
	return db.instruments, db.buildPairs(widthInt), db.minTick, nil
}

func printSummary(book *ledger, results []boxResult, completedCount int, width decimal.Decimal) {
	var totalPnl decimal.Decimal
	for _, br := range results {
		if br.filled == 4 {
			totalPnl = totalPnl.Add(br.pnl)
		}
	}
	log.Printf("  --- summary ---\n  completed: %d  pnl: %s  cash: $%s  recs: %d", completedCount, formatPnl(totalPnl), book.cash.Format(2), records)
}
