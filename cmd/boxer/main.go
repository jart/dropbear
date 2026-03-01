// Command boxer simulates legging into 0DTE SPX box spreads using Databento
// CMBP1 tick-level data. Places limit orders at the NBBO on individual options
// and assembles risk-free box spreads from legs filled via CBOE pro-rata matching.
//
// Usage:
//
//	go run ./cmd/boxer \
//	    -date 2026-02-27 \
//	    -defs ~/databento/SPX/2026-02-27.definition.dbn \
//	    -data ~/databento/SPX/2026-02-27.0dte.cmbp1.dbn \
//	    -v
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"time"
	"unsafe"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
)

type optionInfo struct {
	strike decimal.Decimal
	class  byte // 'C' or 'P'
	symbol string
}

type quote struct {
	bidPx, askPx decimal.Decimal
	bidSz, askSz uint32
	bidPb, askPb databento.Publisher
}

type strikeLevel struct {
	strike        decimal.Decimal
	callID, putID uint32
}

type boxPair struct {
	loStrike, hiStrike decimal.Decimal
	callLoID, callHiID uint32 // C_K1, C_K2
	putLoID, putHiID   uint32 // P_K1, P_K2
}

type pendingOrder struct {
	instID   uint32
	side     byte            // 'B' buy or 'S' sell
	limitPx  decimal.Decimal // our limit price
	cumFill  float64         // accumulated pro-rata fills
	postTime clocky.Time     // when order was posted/repriced
}

type boxLeg struct {
	instID  uint32
	class   byte // 'C' or 'P'
	strike  decimal.Decimal
	side    byte // 'B' bought or 'S' sold
	fillPx  decimal.Decimal
	fillTS  clocky.Time
	orderTS clocky.Time // when order was posted
}

type activeBox struct {
	pair    *boxPair
	legs    [4]*boxLeg       // nil until filled
	orders  [4]*pendingOrder // nil when leg is filled
	filled  int              // count of filled legs (0-4)
	long    bool             // true = long box (buy), false = short box (sell)
	startTS clocky.Time
}

type completedBox struct {
	pair  *boxPair
	legs  [4]boxLeg
	long  bool
	debit decimal.Decimal // net outflow (positive = we paid)
	pnl   decimal.Decimal
}

// dbnPrice converts a Databento int64 price (scale 1e9) to decimal.Decimal (scale 1e6).
func dbnPrice(p int64) decimal.Decimal {
	return decimal.Decimal(p / 1000)
}

func formatET(t clocky.Time) string {
	u := time.Unix(int64(t)/1e9, int64(t)%1e9).In(clocky.NYC)
	return u.Format("15:04:05")
}

func formatPnl(d decimal.Decimal) string {
	if d.IsNegative() {
		return "-$" + d.Neg().Format(2)
	}
	return "+$" + d.Format(2)
}

func main() {
	dateStr := flag.String("date", "2026-02-27", "trading date")
	defsPath := flag.String("defs", "", "path to definitions .dbn")
	dataPath := flag.String("data", "", "path to 0DTE CMBP1 .dbn")
	widthInt := flag.Int("width", 50, "box width in SPX points")
	edgeStr := flag.String("edge", "0.10", "minimum edge per share after commission")
	maxOpen := flag.Int("maxopen", 5, "max active (incomplete) boxes at once")
	startStr := flag.String("start", "09:45", "earliest time to start legging (HH:MM ET)")
	cutoffStr := flag.String("cutoff", "15:30", "stop opening new boxes after this time")
	verbose := flag.Bool("v", false, "verbose output")
	flag.Parse()

	if *defsPath == "" || *dataPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Parse parameters.
	date, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		log.Fatalf("bad date %q: %v", *dateStr, err)
	}
	queryDateInt := date.Year()*10000 + int(date.Month())*100 + date.Day()

	width := decimal.FromInt(*widthInt)
	minEdge := decimal.Parse(*edgeStr)
	commission := decimal.Parse("1.22").MulInt(4) // $4.88 per box
	commPerShare := commission.Div(width)         // $0.0488 per share for 50-wide

	st, err := time.Parse("15:04", *startStr)
	if err != nil {
		log.Fatalf("bad start time %q: %v", *startStr, err)
	}
	startET := clocky.Time(time.Date(date.Year(), date.Month(), date.Day(),
		st.Hour(), st.Minute(), 0, 0, clocky.NYC).UnixNano())

	ct, err := time.Parse("15:04", *cutoffStr)
	if err != nil {
		log.Fatalf("bad cutoff time %q: %v", *cutoffStr, err)
	}
	cutoffET := clocky.Time(time.Date(date.Year(), date.Month(), date.Day(),
		ct.Hour(), ct.Minute(), 0, 0, clocky.NYC).UnixNano())

	expiryET := clocky.Time(time.Date(date.Year(), date.Month(), date.Day(),
		16, 0, 0, 0, clocky.NYC).UnixNano())

	// Max unrealized loss on partial box before cutting.
	maxPartialLoss := minEdge.MulInt(2)

	// Phase 1: Load definitions and build box pairs.
	instruments, pairs, err := loadDefinitions(*defsPath, queryDateInt, *widthInt)
	if err != nil {
		log.Fatalf("load definitions: %v", err)
	}
	if *verbose {
		fmt.Printf("loaded %d instruments, %d box pairs (width=%d)\n",
			len(instruments), len(pairs), *widthInt)
	}

	// Build lookup: instrument ID → list of box pairs containing it.
	pairsByInst := make(map[uint32][]*boxPair)
	for i := range pairs {
		p := &pairs[i]
		for _, id := range []uint32{p.callLoID, p.callHiID, p.putLoID, p.putHiID} {
			pairsByInst[id] = append(pairsByInst[id], p)
		}
	}

	// Phase 2: Stream CMBP1 and simulate legging.
	quotes := make(map[uint32]*quote)
	var active []*activeBox
	var completed []completedBox
	var partialPnl decimal.Decimal

	f, err := os.Open(*dataPath)
	if err != nil {
		log.Fatalf("open data: %v", err)
	}
	defer f.Close()

	meta, err := databento.DecodeMetadata(f)
	if err != nil {
		log.Fatalf("decode metadata: %v", err)
	}

	br := bufio.NewReaderSize(f, 2<<20) // 2MB buffer for 24GB file
	records := 0
	cmbp1Size := int(unsafe.Sizeof(databento.CMBP1{}))

	for {
		rec, err := databento.DecodeRecord(br)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("decode record: %v", err)
		}
		rec, err = databento.UpgradeRecord(meta.Version, rec)
		if err != nil {
			log.Fatalf("upgrade record: %v", err)
		}
		if len(rec) < cmbp1Size {
			continue
		}
		tick := (*databento.CMBP1)(unsafe.Pointer(&rec[0]))
		records++

		instID := tick.Header.InstrumentID
		if _, ok := instruments[instID]; !ok {
			continue
		}

		// Update quote.
		bidPx := tick.Levels[0].BidPx
		askPx := tick.Levels[0].AskPx
		if bidPx == databento.UndefPrice || askPx == databento.UndefPrice {
			continue
		}
		q, ok := quotes[instID]
		if !ok {
			q = &quote{}
			quotes[instID] = q
		}
		q.bidPx = dbnPrice(bidPx)
		q.askPx = dbnPrice(askPx)
		q.bidSz = tick.Levels[0].BidSz
		q.askSz = tick.Levels[0].AskSz
		q.bidPb = tick.Levels[0].BidPb
		q.askPb = tick.Levels[0].AskPb

		ts := tick.Header.TSEvent

		if ts.Before(startET) {
			continue
		}

		// Process fills on active boxes.
		if tick.Action == databento.ActionTrade {
			tradePx := dbnPrice(tick.Price)
			tradeSz := tick.Size
			for _, ab := range active {
				for i, ord := range ab.orders {
					if ord == nil || ord.instID != instID {
						continue
					}
					if tradePx.Cmp(ord.limitPx) != 0 {
						continue
					}
					// Check CBOE is at NBBO on our side.
					var queueSz uint32
					if ord.side == 'B' {
						if q.bidPb == databento.PublisherOpraPillarXcbo {
							queueSz = q.bidSz
						}
					} else {
						if q.askPb == databento.PublisherOpraPillarXcbo {
							queueSz = q.askSz
						}
					}
					if queueSz == 0 {
						continue
					}
					ord.cumFill += float64(tradeSz) / float64(queueSz)
					if ord.cumFill >= 1.0 {
						// Filled!
						info := instruments[instID]
						ab.legs[i] = &boxLeg{
							instID:  instID,
							class:   info.class,
							strike:  info.strike,
							side:    ord.side,
							fillPx:  ord.limitPx,
							fillTS:  ts,
							orderTS: ord.postTime,
						}
						ab.orders[i] = nil
						ab.filled++
						if *verbose {
							side := "Buy "
							if ord.side == 'S' {
								side = "Sell"
							}
							fillDur := time.Duration(ts-ord.postTime) * time.Nanosecond
							fmt.Printf("    %s %c%-5d %s @ %-8s (fill time: %s)\n",
								side, info.class, info.strike.Int(),
								formatET(ts), ord.limitPx.Format(2),
								fillDur.Truncate(time.Second))
						}
					}
				}
			}
		}

		// Reprice pending orders when NBBO changes.
		for _, ab := range active {
			for _, ord := range ab.orders {
				if ord == nil || ord.instID != instID {
					continue
				}
				var newPx decimal.Decimal
				if ord.side == 'B' {
					newPx = q.bidPx
				} else {
					newPx = q.askPx
				}
				if newPx.Cmp(ord.limitPx) != 0 {
					ord.limitPx = newPx
					ord.cumFill = 0
					ord.postTime = ts
				}
			}
		}

		// Check for completed boxes.
		j := 0
		for _, ab := range active {
			if ab.filled == 4 {
				cb := settleBox(ab, width, commPerShare)
				completed = append(completed, cb)
				if *verbose {
					dir := "LONG"
					if !cb.long {
						dir = "SHORT"
					}
					fmt.Printf("  completed %s %d/%d  debit: $%s  pnl: %s/share\n",
						dir, ab.pair.loStrike.Int(), ab.pair.hiStrike.Int(),
						cb.debit.Format(2), formatPnl(cb.pnl))
				}
				continue
			}
			active[j] = ab
			j++
		}
		active = active[:j]

		// Risk management: cut partial boxes with too much unrealized loss.
		j = 0
		for _, ab := range active {
			if ab.filled > 0 && ab.filled < 4 {
				loss := partialMTM(ab, quotes, width)
				if loss.Neg().Cmp(maxPartialLoss.MulInt(ab.filled)) > 0 {
					// Cut: cross spread to close filled legs.
					pl := closePartial(ab, quotes)
					partialPnl = partialPnl.Add(pl)
					if *verbose {
						fmt.Printf("  CUT partial %d/%d (%d legs) P&L: %s\n",
							ab.pair.loStrike.Int(), ab.pair.hiStrike.Int(),
							ab.filled, formatPnl(pl))
					}
					continue
				}
			}
			active[j] = ab
			j++
		}
		active = active[:j]

		// Past expiry: settle everything.
		if !ts.Before(expiryET) {
			for _, ab := range active {
				if ab.filled == 4 {
					cb := settleBox(ab, width, commPerShare)
					completed = append(completed, cb)
				} else if ab.filled > 0 {
					pl := closePartial(ab, quotes)
					partialPnl = partialPnl.Add(pl)
				}
			}
			active = active[:0]
			break
		}

		// Past cutoff: don't open new boxes.
		if !ts.Before(cutoffET) {
			continue
		}

		// Scan for new box opportunities.
		if len(active) >= *maxOpen {
			continue
		}

		// Only scan when this tick's instrument is part of a box pair.
		instPairs := pairsByInst[instID]
		if len(instPairs) == 0 {
			continue
		}

		for _, bp := range instPairs {
			if len(active) >= *maxOpen {
				break
			}
			// Skip if we already have an active box on this pair.
			if hasActivePair(active, bp) {
				continue
			}
			// Need all 4 quotes.
			qCL := quotes[bp.callLoID]
			qCH := quotes[bp.callHiID]
			qPL := quotes[bp.putLoID]
			qPH := quotes[bp.putHiID]
			if qCL == nil || qCH == nil || qPL == nil || qPH == nil {
				continue
			}
			if qCL.bidPx.IsZero() || qCH.bidPx.IsZero() ||
				qPL.bidPx.IsZero() || qPH.bidPx.IsZero() {
				continue
			}

			// Long box: buy at bids, sell at asks.
			// debit = bid(C_K1) + bid(P_K2) - ask(C_K2) - ask(P_K1)
			longDebit := qCL.bidPx.Add(qPH.bidPx).Sub(qCH.askPx).Sub(qPL.askPx)
			longEdge := width.Sub(longDebit).Sub(commPerShare)

			// Short box: sell at asks, buy at bids.
			// credit = ask(C_K1) + ask(P_K2) - bid(C_K2) - bid(P_K1)
			shortCredit := qCL.askPx.Add(qPH.askPx).Sub(qCH.bidPx).Sub(qPL.bidPx)
			shortEdge := shortCredit.Sub(width).Sub(commPerShare)

			if longEdge.Cmp(minEdge) >= 0 && longEdge.Cmp(shortEdge) >= 0 {
				ab := startBox(bp, true, quotes, ts)
				active = append(active, ab)
				if *verbose {
					fmt.Printf("\n  box #%d LONG %d/%d  est.debit: $%s  est.edge: %s\n",
						len(completed)+len(active),
						bp.loStrike.Int(), bp.hiStrike.Int(),
						longDebit.Format(2), formatPnl(longEdge))
				}
			} else if shortEdge.Cmp(minEdge) >= 0 {
				ab := startBox(bp, false, quotes, ts)
				active = append(active, ab)
				if *verbose {
					fmt.Printf("\n  box #%d SHORT %d/%d  est.credit: $%s  est.edge: %s\n",
						len(completed)+len(active),
						bp.loStrike.Int(), bp.hiStrike.Int(),
						shortCredit.Format(2), formatPnl(shortEdge))
				}
			}
		}

	}

	// Settle any remaining active boxes at close.
	for _, ab := range active {
		if ab.filled == 4 {
			cb := settleBox(ab, width, commPerShare)
			completed = append(completed, cb)
		} else if ab.filled > 0 {
			pl := closePartial(ab, quotes)
			partialPnl = partialPnl.Add(pl)
			if *verbose {
				fmt.Printf("  close partial %d/%d (%d legs) at expiry: %s\n",
					ab.pair.loStrike.Int(), ab.pair.hiStrike.Int(),
					ab.filled, formatPnl(pl))
			}
		}
	}

	// Print results.
	fmt.Printf("%s 0DTE box spread legging\n", *dateStr)
	fmt.Printf("  parameters: width=%d  edge=%s  maxopen=%d\n",
		*widthInt, minEdge.String(), *maxOpen)
	fmt.Printf("  commission: $%s/box ($%s/share)\n",
		commission.Format(2), commPerShare.Format(4))
	fmt.Println()

	var totalPnl decimal.Decimal
	var totalFillTime time.Duration
	var totalLegs int
	var longCount, shortCount int

	for i, cb := range completed {
		dir := "LONG"
		if !cb.long {
			dir = "SHORT"
			shortCount++
		} else {
			longCount++
		}
		fmt.Printf("  box #%d %s %d/%d\n", i+1, dir,
			cb.pair.loStrike.Int(), cb.pair.hiStrike.Int())
		for _, leg := range cb.legs {
			side := "Buy "
			if leg.side == 'S' {
				side = "Sell"
			}
			fillDur := time.Duration(leg.fillTS-leg.orderTS) * time.Nanosecond
			totalFillTime += fillDur
			totalLegs++
			fmt.Printf("    %s %c%-5d %s @ %-8s (fill time: %s)\n",
				side, leg.class, leg.strike.Int(),
				formatET(leg.fillTS), leg.fillPx.Format(2),
				fillDur.Truncate(time.Second))
		}
		fmt.Printf("    debit: $%s  profit: %s/share (%s/contract)\n",
			cb.debit.Format(2), formatPnl(cb.pnl),
			formatPnl(cb.pnl.MulInt(100)))
		totalPnl = totalPnl.Add(cb.pnl)
	}

	fmt.Println()
	fmt.Println("  --- summary ---")
	fmt.Printf("  completed boxes: %d (%d long, %d short)\n",
		len(completed), longCount, shortCount)
	fmt.Printf("  total P&L: %s/share (%s/contract)\n",
		formatPnl(totalPnl), formatPnl(totalPnl.MulInt(100)))
	if totalLegs > 0 {
		avgFill := totalFillTime / time.Duration(totalLegs)
		fmt.Printf("  avg fill time: %s per leg\n", avgFill.Truncate(time.Second))
	}
	if !partialPnl.IsZero() {
		fmt.Printf("  partial boxes P&L: %s\n", formatPnl(partialPnl))
	}
	netPnl := totalPnl.Add(partialPnl)
	fmt.Printf("  net P&L: %s/share (%s/contract)\n",
		formatPnl(netPnl), formatPnl(netPnl.MulInt(100)))
	totalComm := commPerShare.MulInt(len(completed))
	fmt.Printf("  commission: $%s\n", totalComm.Format(2))
	fmt.Printf("  records: %d\n", records)
}

// startBox creates an activeBox and posts limit orders for all 4 legs.
func startBox(bp *boxPair, long bool, quotes map[uint32]*quote, ts clocky.Time) *activeBox {
	ab := &activeBox{
		pair:    bp,
		long:    long,
		startTS: ts,
	}
	// Long box: Buy C_K1 at bid, Sell C_K2 at ask, Buy P_K2 at bid, Sell P_K1 at ask
	// Short box: Sell C_K1 at ask, Buy C_K2 at bid, Sell P_K2 at ask, Buy P_K1 at bid
	type legSpec struct {
		instID uint32
		side   byte // 'B' or 'S'
	}
	var specs [4]legSpec
	if long {
		specs = [4]legSpec{
			{bp.callLoID, 'B'}, // Buy C_K1
			{bp.callHiID, 'S'}, // Sell C_K2
			{bp.putHiID, 'B'},  // Buy P_K2
			{bp.putLoID, 'S'},  // Sell P_K1
		}
	} else {
		specs = [4]legSpec{
			{bp.callLoID, 'S'}, // Sell C_K1
			{bp.callHiID, 'B'}, // Buy C_K2
			{bp.putHiID, 'S'},  // Sell P_K2
			{bp.putLoID, 'B'},  // Buy P_K1
		}
	}
	for i, s := range specs {
		q := quotes[s.instID]
		var px decimal.Decimal
		if s.side == 'B' {
			px = q.bidPx
		} else {
			px = q.askPx
		}
		ab.orders[i] = &pendingOrder{
			instID:   s.instID,
			side:     s.side,
			limitPx:  px,
			postTime: ts,
		}
	}
	return ab
}

// settleBox settles a fully filled box at expiration.
func settleBox(ab *activeBox, width, commPerShare decimal.Decimal) completedBox {
	cb := completedBox{
		pair: ab.pair,
		long: ab.long,
	}
	var netOutflow decimal.Decimal
	for i, leg := range ab.legs {
		cb.legs[i] = *leg
		if leg.side == 'B' {
			netOutflow = netOutflow.Add(leg.fillPx)
		} else {
			netOutflow = netOutflow.Sub(leg.fillPx)
		}
	}
	cb.debit = netOutflow
	// Long box: profit = width - debit - commission
	// Short box: profit = -debit - width - commission (debit is negative = credit)
	//   which is: credit - width - commission = -debit - width - commission
	if ab.long {
		cb.pnl = width.Sub(netOutflow).Sub(commPerShare)
	} else {
		cb.pnl = netOutflow.Neg().Sub(width).Sub(commPerShare)
	}
	return cb
}

// partialMTM computes unrealized P&L on a partial box's filled legs.
func partialMTM(ab *activeBox, quotes map[uint32]*quote, width decimal.Decimal) decimal.Decimal {
	var mtm decimal.Decimal
	for _, leg := range ab.legs {
		if leg == nil {
			continue
		}
		q := quotes[leg.instID]
		if q == nil {
			continue
		}
		mid := q.bidPx.Add(q.askPx).DivInt(2)
		if leg.side == 'B' {
			// We bought at fillPx, current value is mid.
			mtm = mtm.Add(mid.Sub(leg.fillPx))
		} else {
			// We sold at fillPx, current value is mid.
			mtm = mtm.Add(leg.fillPx.Sub(mid))
		}
	}
	return mtm
}

// closePartial closes filled legs at market (crossing spread) and returns P&L.
func closePartial(ab *activeBox, quotes map[uint32]*quote) decimal.Decimal {
	var pnl decimal.Decimal
	for _, leg := range ab.legs {
		if leg == nil {
			continue
		}
		q := quotes[leg.instID]
		if q == nil {
			continue
		}
		if leg.side == 'B' {
			// Close long by selling at bid.
			pnl = pnl.Add(q.bidPx.Sub(leg.fillPx))
		} else {
			// Close short by buying at ask.
			pnl = pnl.Add(leg.fillPx.Sub(q.askPx))
		}
	}
	// Commission: $1.22/leg round trip (open + close = 2 per filled leg).
	pnl = pnl.Sub(decimal.Parse("1.22").MulInt(ab.filled * 2))
	return pnl
}

func hasActivePair(active []*activeBox, bp *boxPair) bool {
	for _, ab := range active {
		if ab.pair == bp {
			return true
		}
	}
	return false
}

// loadDefinitions reads a definitions .dbn file and returns instruments,
// and box pairs for options expiring on the given date with the given width.
func loadDefinitions(path string, queryDateInt, widthInt int) (map[uint32]optionInfo, []boxPair, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	meta, err := databento.DecodeMetadata(f)
	if err != nil {
		return nil, nil, fmt.Errorf("decode metadata: %w", err)
	}

	instruments := make(map[uint32]optionInfo)
	strikeMap := make(map[decimal.Decimal]*strikeLevel)
	instSize := int(unsafe.Sizeof(databento.Instrument{}))

	for {
		rec, err := databento.DecodeRecord(f)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, fmt.Errorf("decode record: %w", err)
		}
		rec, err = databento.UpgradeRecord(meta.Version, rec)
		if err != nil {
			return nil, nil, fmt.Errorf("upgrade record: %w", err)
		}
		if len(rec) < instSize {
			continue
		}
		inst := (*databento.Instrument)(unsafe.Pointer(&rec[0]))

		var class byte
		switch inst.InstrumentClass {
		case databento.InstrumentClassCall:
			class = 'C'
		case databento.InstrumentClassPut:
			class = 'P'
		default:
			continue
		}

		expUTC := time.Unix(int64(inst.Expiration)/1e9, int64(inst.Expiration)%1e9).UTC()
		expDateInt := expUTC.Year()*10000 + int(expUTC.Month())*100 + expUTC.Day()
		if expDateInt != queryDateInt {
			continue
		}

		if inst.StrikePrice == databento.UndefPrice {
			continue
		}

		strike := dbnPrice(inst.StrikePrice)
		id := inst.Header.InstrumentID
		instruments[id] = optionInfo{
			strike: strike,
			class:  class,
			symbol: inst.GetRawSymbol(),
		}

		sl, ok := strikeMap[strike]
		if !ok {
			sl = &strikeLevel{strike: strike}
			strikeMap[strike] = sl
		}
		if class == 'C' {
			sl.callID = id
		} else {
			sl.putID = id
		}
	}

	// Build sorted strikes (only those with both call and put).
	strikes := make([]strikeLevel, 0, len(strikeMap))
	for _, sl := range strikeMap {
		if sl.callID != 0 && sl.putID != 0 {
			strikes = append(strikes, *sl)
		}
	}
	sort.Slice(strikes, func(i, j int) bool {
		return strikes[i].strike.Cmp(strikes[j].strike) < 0
	})

	// Build box pairs: every (K1, K2) where K2 = K1 + width.
	widthDec := decimal.FromInt(widthInt)
	var pairs []boxPair
	for i, lo := range strikes {
		target := lo.strike.Add(widthDec)
		// Binary search for the hi strike.
		hi := sort.Search(len(strikes)-i-1, func(j int) bool {
			return strikes[i+1+j].strike.Cmp(target) >= 0
		})
		hi += i + 1
		if hi < len(strikes) && strikes[hi].strike.Cmp(target) == 0 {
			pairs = append(pairs, boxPair{
				loStrike: lo.strike,
				hiStrike: strikes[hi].strike,
				callLoID: lo.callID,
				callHiID: strikes[hi].callID,
				putLoID:  lo.putID,
				putHiID:  strikes[hi].putID,
			})
		}
	}

	return instruments, pairs, nil
}
