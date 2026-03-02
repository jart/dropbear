// Command boxer legs into 0DTE box spreads using Databento CMBP1 tick data.
// Works with any optionable underlying (SPX, SPY, TSLA, etc). Supports both
// backtesting from DBN files and live paper trading via Databento's LSG.
//
// Backtest:
//
//	go run ./cmd/boxer \
//	    -defs ~/databento/SPX/2026-02-27.definition.dbn \
//	    -data ~/databento/SPX/2026-02-27.0dte.cmbp1.dbn \
//	    -width 50 -v
//
// Live (paper trade):
//
//	go run ./cmd/boxer -live -symbol SPXW -width 50 -v
//	go run ./cmd/boxer -live -symbol SPY -width 5 -tick 0.01 -v
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"time"
	"unsafe"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
	"dropbear/netty"
)

// Set from flags in main().
var (
	tick       decimal.Decimal // minimum price increment
	multiplier int             // contract multiplier (shares per contract)
	records    int             // total records seen
	lastDefTS  clocky.Time     // for throttling addInstrument logging
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
	instID    uint32
	side      byte            // 'B' buy or 'S' sell
	limitPx   decimal.Decimal // live limit price on $0.05 grid (per share)
	postTime  clocky.Time     // when limitPx became effective
	pendingPx decimal.Decimal // new price from modifyOrder (zero = no pending modify)
	modifyTS  clocky.Time     // when modifyOrder was called
}

type boxLeg struct {
	instID  uint32
	class   byte // 'C' or 'P'
	strike  decimal.Decimal
	side    byte            // 'B' bought or 'S' sold
	fillPx  decimal.Decimal // per share
	fillTS  clocky.Time
	orderTS clocky.Time
}

type activeBox struct {
	pair    *boxPair
	legs    [4]*boxLeg       // nil until filled
	orders  [4]*pendingOrder // nil when leg is filled
	filled  int
	long    bool
	startTS clocky.Time
}

// -----------------------------------------------------------------------
// Order book / trade blotter — the accounting ledger.
// -----------------------------------------------------------------------

type fill struct {
	ts     clocky.Time
	instID uint32
	side   byte            // 'B' or 'S'
	qty    int             // always 1 for us
	px     decimal.Decimal // per share
	cash   decimal.Decimal // signed cash flow (negative = spent, positive = received)
}

type ledger struct {
	fills    []fill
	cash     decimal.Decimal            // running cash balance
	position map[uint32]int             // net position per instrument (long positive)
	cost     map[uint32]decimal.Decimal // total cost basis per instrument
}

func newLedger(startingCash decimal.Decimal) *ledger {
	return &ledger{
		cash:     startingCash,
		position: make(map[uint32]int),
		cost:     make(map[uint32]decimal.Decimal),
	}
}

// recordFill books a fill into the ledger. qty is always 1 contract.
func (l *ledger) recordFill(ts clocky.Time, instID uint32, side byte, px decimal.Decimal) {
	// Cash flow: buy costs money, sell receives money.
	// Per contract = px * multiplier.
	cashFlow := px.MulInt(multiplier)
	if side == 'B' {
		cashFlow = cashFlow.Neg() // spent
		l.position[instID]++
		l.cost[instID] = l.cost[instID].Add(px)
	} else {
		l.position[instID]--
		l.cost[instID] = l.cost[instID].Sub(px)
	}
	l.cash = l.cash.Add(cashFlow)
	l.fills = append(l.fills, fill{
		ts:     ts,
		instID: instID,
		side:   side,
		qty:    1,
		px:     px,
		cash:   cashFlow,
	})
}

// settle expires an instrument at intrinsic value. If we're long, we receive
// intrinsic × multiplier. If short, we pay it.
func (l *ledger) settle(instID uint32, intrinsic decimal.Decimal) decimal.Decimal {
	pos := l.position[instID]
	if pos == 0 {
		return decimal.Zero
	}
	// Cash = position × intrinsic × multiplier
	cashFlow := intrinsic.MulInt(pos * multiplier)
	l.cash = l.cash.Add(cashFlow)
	l.position[instID] = 0
	return cashFlow
}

// payCommission deducts commission from cash.
func (l *ledger) payCommission(amount decimal.Decimal) {
	l.cash = l.cash.Sub(amount)
}

// -----------------------------------------------------------------------
// Broker interface — replace these for live trading.
// -----------------------------------------------------------------------

// broker tracks rate limiting for the broker API using a sliding window
// of call timestamps. Works with simulated time for backtesting.
type broker struct {
	calls []clocky.Time // circular buffer of timestamps
	head  int           // next write position
	limit int           // max calls per minute
}

func newBroker(limitPerMinute int) *broker {
	return &broker{
		calls: make([]clocky.Time, limitPerMinute),
		limit: limitPerMinute,
	}
}

// canCall returns true if n broker API calls are allowed at time ts
// without exceeding the per-minute rate limit.
func (b *broker) canCall(ts clocky.Time, n int) bool {
	// Check the n-th oldest call (the one that would be evicted last).
	idx := (b.head + n - 1) % b.limit
	oldest := b.calls[idx]
	return oldest == 0 || clocky.Duration(ts-oldest) >= clocky.Minute
}

// recordCall records a broker API call at time ts.
func (b *broker) recordCall(ts clocky.Time) {
	b.calls[b.head] = ts
	b.head = (b.head + 1) % b.limit
}

// limitOrder places a passive limit order at the specified price.
func (b *broker) limitOrder(instID uint32, side byte, px decimal.Decimal, ts clocky.Time) *pendingOrder {
	b.recordCall(ts)
	return &pendingOrder{
		instID:   instID,
		side:     side,
		limitPx:  px,
		postTime: ts,
	}
}

// modifyOrder requests a limit price change. The old price remains live
// until the modify arrives (after latency). During that window the order
// can still fill at the old price.
func (b *broker) modifyOrder(ord *pendingOrder, newPx decimal.Decimal, ts clocky.Time) {
	b.recordCall(ts)
	ord.pendingPx = newPx
	ord.modifyTS = ts
}

// cancelOrder cancels a pending limit order. No-op in backtest.
func (b *broker) cancelOrder(_ *pendingOrder) {
	// In live trading this would call the broker API and consume a rate limit token.
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func dbnPrice(p int64) decimal.Decimal {
	return decimal.Decimal(p / 1000)
}

func formatET(t clocky.Time) string {
	u := time.Unix(int64(t)/1e9, int64(t)%1e9).In(clocky.MTV)
	return u.Format("15:04:05.000")
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Truncate(time.Second).String()
}

func formatPnl(d decimal.Decimal) string {
	if d.IsNegative() {
		return "-$" + d.Neg().Format(2)
	}
	return "+$" + d.Format(2)
}

func snapBid(px decimal.Decimal) decimal.Decimal {
	return px.QuantizeTruncate(tick)
}

func snapAsk(px decimal.Decimal) decimal.Decimal {
	return px.QuantizeAway(tick)
}

// greedyMid computes a greed-adjusted limit price. greed=0 returns mid,
// greed=1 returns the favorable side of the spread (bid for buy, ask for sell).
// The result is snapped to the tick grid conservatively for each side.
func greedyMid(q *quote, side byte, greed decimal.Decimal) decimal.Decimal {
	mid := q.bidPx.Add(q.askPx).DivInt(2)
	halfSpread := q.askPx.Sub(q.bidPx).DivInt(2)
	offset := greed.Mul(halfSpread)
	if side == 'B' {
		// Greedy buy: pull limit below mid towards bid.
		return mid.Sub(offset).QuantizeAway(tick)
	}
	// Greedy sell: push limit above mid towards ask.
	return mid.Add(offset).QuantizeTruncate(tick)
}

// computeGreed returns the effective greed level based on time elapsed since
// the box was opened and how many legs have already filled.
// Greed decays linearly over patience, and is scaled by unfilled legs.
func computeGreed(initialGreed decimal.Decimal, elapsed, patience clocky.Duration, legsFilled int) decimal.Decimal {
	if patience <= 0 {
		return decimal.Zero
	}
	if elapsed >= patience {
		return decimal.Zero
	}
	// timeDecay = (patience - elapsed) / patience, using milliseconds to avoid overflow.
	remainMs := int((patience - elapsed) / clocky.Millisecond)
	totalMs := int(patience / clocky.Millisecond)
	if totalMs <= 0 {
		return decimal.Zero
	}
	timeDecay := decimal.FromInt(remainMs).DivInt(totalMs)
	// legDecay = (4 - filled) / 4
	legDecay := decimal.FromInt(4 - legsFilled).DivInt(4)
	return initialGreed.Mul(timeDecay).Mul(legDecay)
}

// budgetLimit computes the worst acceptable limit price for a single leg,
// given already-filled legs and assuming other unfilled legs fill at mid.
// Returns the max buy price or min sell price that keeps the box profitable
// by at least minEdge per share.
func budgetLimit(ab *activeBox, legIdx int, width, minEdge decimal.Decimal, quotes map[uint32]*quote) decimal.Decimal {
	// Sum debit from already-filled legs.
	var debit decimal.Decimal
	for _, leg := range ab.legs {
		if leg == nil {
			continue
		}
		if leg.side == 'B' {
			debit = debit.Add(leg.fillPx)
		} else {
			debit = debit.Sub(leg.fillPx)
		}
	}

	// Assume other unfilled legs fill at their current mid (zero improvement).
	for i, ord := range ab.orders {
		if ord == nil || i == legIdx {
			continue
		}
		q := quotes[ord.instID]
		if q == nil || q.bidPx.IsZero() || q.askPx.IsZero() {
			continue
		}
		mid := q.bidPx.Add(q.askPx).DivInt(2)
		if ord.side == 'B' {
			debit = debit.Add(mid)
		} else {
			debit = debit.Sub(mid)
		}
	}

	// For LONG: totalDebit <= width - minEdge
	//   BUY leg adds +px → px <= width - minEdge - debit
	//   SELL leg adds -px → -px <= width - minEdge - debit → px >= debit - width + minEdge
	// For SHORT: totalDebit <= -(width + minEdge)
	//   BUY leg adds +px → px <= -(width + minEdge) - debit
	//   SELL leg adds -px → px >= debit + width + minEdge
	ord := ab.orders[legIdx]
	if ab.long {
		room := width.Sub(minEdge).Sub(debit)
		if ord.side == 'B' {
			return snapBid(room) // max buy price
		}
		return snapAsk(room.Neg()) // min sell price (room is what we can afford to add)
	}
	room := width.Add(minEdge).Neg().Sub(debit)
	if ord.side == 'B' {
		return snapBid(room) // max buy price
	}
	return snapAsk(room.Neg()) // min sell price
}

// clampLimit restricts a greedy limit price to the budget limit.
// For buys: limit = min(greedy, budget). For sells: limit = max(greedy, budget).
func clampLimit(side byte, greedy, budget decimal.Decimal) decimal.Decimal {
	if side == 'B' {
		return greedy.Min(budget)
	}
	return greedy.Max(budget)
}

func legQualified(q *quote, minBid, maxSpread decimal.Decimal) bool {
	if q.bidPx.Cmp(minBid) < 0 {
		return false
	}
	if q.askPx.Cmp(q.bidPx) <= 0 {
		return false
	}
	return q.askPx.Sub(q.bidPx).Cmp(maxSpread) <= 0
}

func hasActivePair(active []*activeBox, bp *boxPair) bool {
	for _, ab := range active {
		if ab.pair == bp {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------
// Box operations
// -----------------------------------------------------------------------

// legSpecs returns the 4 (instID, side) pairs for a box.
func legSpecs(bp *boxPair, long bool) [4]struct {
	instID uint32
	side   byte
} {
	if long {
		return [4]struct {
			instID uint32
			side   byte
		}{
			{bp.callLoID, 'B'}, // Buy C_K1
			{bp.callHiID, 'S'}, // Sell C_K2
			{bp.putHiID, 'B'},  // Buy P_K2
			{bp.putLoID, 'S'},  // Sell P_K1
		}
	}
	return [4]struct {
		instID uint32
		side   byte
	}{
		{bp.callLoID, 'S'}, // Sell C_K1
		{bp.callHiID, 'B'}, // Buy C_K2
		{bp.putHiID, 'S'},  // Sell P_K2
		{bp.putLoID, 'B'},  // Buy P_K1
	}
}

func startBox(b *broker, bp *boxPair, long bool, quotes map[uint32]*quote, ts clocky.Time, greed, width, minEdge decimal.Decimal) *activeBox {
	ab := &activeBox{
		pair:    bp,
		long:    long,
		startTS: ts,
	}
	specs := legSpecs(bp, long)
	for i, s := range specs {
		q := quotes[s.instID]
		px := greedyMid(q, s.side, greed)
		ab.orders[i] = b.limitOrder(s.instID, s.side, px, ts)
	}
	// Clamp each limit to the budget so no fill can make the box unprofitable.
	for i, ord := range ab.orders {
		if ord == nil {
			continue
		}
		budget := budgetLimit(ab, i, width, minEdge, quotes)
		ord.limitPx = clampLimit(ord.side, ord.limitPx, budget)
	}
	return ab
}

// cancelBox cancels all pending orders on a box.
func cancelBox(b *broker, ab *activeBox) {
	for i, ord := range ab.orders {
		if ord != nil {
			b.cancelOrder(ord)
			ab.orders[i] = nil
		}
	}
}

// -----------------------------------------------------------------------
// Display
// -----------------------------------------------------------------------

func printLegQuotes(bp *boxPair, quotes map[uint32]*quote) {
	type legInfo struct {
		name string
		id   uint32
	}
	legs := []legInfo{
		{fmt.Sprintf("C%d", bp.loStrike.Int()), bp.callLoID},
		{fmt.Sprintf("C%d", bp.hiStrike.Int()), bp.callHiID},
		{fmt.Sprintf("P%d", bp.loStrike.Int()), bp.putLoID},
		{fmt.Sprintf("P%d", bp.hiStrike.Int()), bp.putHiID},
	}
	for _, l := range legs {
		q := quotes[l.id]
		if q == nil {
			log.Printf("      %-8s no quote", l.name)
			continue
		}
		log.Printf("      %-8s %s × %-4d / %s × %-4d  sprd %s",
			l.name,
			q.bidPx.Format(2), q.bidSz,
			q.askPx.Format(2), q.askSz,
			q.askPx.Sub(q.bidPx).Format(2))
	}
}

// -----------------------------------------------------------------------
// Main
// -----------------------------------------------------------------------

func main() {
	live := flag.Bool("live", false, "live paper trading mode (streams from Databento LSG)")
	symbol := flag.String("symbol", "", "parent symbol for live mode (e.g. SPXW, SPY, TSLA)")
	dataset := flag.String("dataset", "OPRA.PILLAR", "Databento dataset for live mode")
	dateStr := flag.String("date", "", "trading date (default: today)")
	defsPath := flag.String("defs", "", "path to definitions .dbn (backtest)")
	dataPath := flag.String("data", "", "path to 0DTE CMBP1 .dbn (backtest)")
	widthInt := flag.Int("width", 50, "box width in strike points")
	tickFlag := decimal.Flag("tick", "0.05", "minimum price increment")
	mult := flag.Int("mult", 100, "contract multiplier (shares per contract)")
	commStr := flag.String("comm", "1.22", "commission per contract per leg")
	edgeStr := flag.String("edge", "0.10", "minimum edge per share")
	maxEdgeStr := flag.String("maxedge", "2.00", "max edge per share")
	maxSpreadStr := flag.String("maxspread", "1.00", "max bid-ask spread per leg")
	minBidStr := flag.String("minbid", "0.05", "minimum bid on each leg")
	maxOpen := flag.Int("maxopen", 4, "max active (incomplete) boxes at once")
	latencyFlag := clocky.DurationFlag("latency", "70ms", "simulated order latency")
	patienceFlag := clocky.DurationFlag("patience", "500ms", "time to decay from full greed to zero")
	greedFlag := decimal.Flag("greed", "0", "initial greed: 0=mid, 1=favorable side of spread")
	rateLimit := flag.Int("ratelimit", 200, "max broker API requests per minute")
	startStr := flag.String("start", "06:30:05", "earliest time to start legging (HH:MM:SS PT)")
	cutoffStr := flag.String("cutoff", "07:30:00", "stop opening new boxes after this time (HH:MM:SS PT)")
	cashInt := flag.Int("cash", 100000, "starting cash in dollars")
	verbose := flag.Bool("v", false, "verbose output")

	loggy.Init()
	flag.Parse()

	if !*live {
		netty.SetOffline()
		clocky.Now = clocky.FakeNow
		clocky.Sleep = clocky.FakeSleep
		clocky.NewTicker = clocky.FakeNewTicker
		if *defsPath == "" || *dataPath == "" {
			log.Fatalf("backtest mode requires -defs and -data flags")
		}
	} else {
		if *symbol == "" {
			log.Fatalf("live mode requires -symbol flag (e.g. -symbol SPXW)")
		}
	}

	var date time.Time
	if *dateStr != "" {
		var err error
		date, err = time.Parse("2006-01-02", *dateStr)
		if err != nil {
			log.Fatalf("bad date %q: %v", *dateStr, err)
		}
	} else {
		date = time.Now().In(clocky.MTV)
	}
	queryDateInt := date.Year()*10000 + int(date.Month())*100 + date.Day()

	multiplier = *mult

	width := decimal.FromInt(*widthInt)
	minEdge := decimal.Parse(*edgeStr)
	maxEdge := decimal.Parse(*maxEdgeStr)
	maxSpread := decimal.Parse(*maxSpreadStr)
	minBid := decimal.Parse(*minBidStr)
	commPerLeg := decimal.Parse(*commStr)
	commPerBox := commPerLeg.MulInt(4)
	commPerSharePerBox := commPerBox.DivInt(multiplier)
	startingCash := decimal.FromInt(*cashInt)

	st, err := time.Parse("15:04:05", *startStr)
	if err != nil {
		log.Fatalf("bad start time %q: %v", *startStr, err)
	}
	startMTV := clocky.Time(time.Date(date.Year(), date.Month(), date.Day(),
		st.Hour(), st.Minute(), st.Second(), 0, clocky.MTV).UnixNano())

	ct, err := time.Parse("15:04:05", *cutoffStr)
	if err != nil {
		log.Fatalf("bad cutoff time %q: %v", *cutoffStr, err)
	}
	cutoffMTV := clocky.Time(time.Date(date.Year(), date.Month(), date.Day(),
		ct.Hour(), ct.Minute(), ct.Second(), 0, clocky.MTV).UnixNano())

	expiryMTV := clocky.Time(time.Date(date.Year(), date.Month(), date.Day(),
		13, 0, 0, 0, clocky.MTV).UnixNano())

	// Phase 1: Load definitions and build box pairs.
	var instruments map[uint32]optionInfo
	var pairs []boxPair
	pairsByInst := make(map[uint32][]*boxPair)
	var liveDB *defBuilder // non-nil in live mode until pairs are built
	lastPairCount := 0 // for logging new pairs in live mode

	if *live {
		tick = *tickFlag
		instruments = make(map[uint32]optionInfo)
		liveDB = newDefBuilder()
	} else {
		var minTick decimal.Decimal
		var err error
		instruments, pairs, minTick, err = loadDefinitions(*defsPath, queryDateInt, *widthInt)
		if err != nil {
			log.Fatalf("load definitions: %v", err)
		}
		if !minTick.IsZero() {
			tick = minTick
		} else {
			tick = *tickFlag
		}
		lastPairCount = len(pairs)
		for i := range pairs {
			p := &pairs[i]
			for _, id := range []uint32{p.callLoID, p.callHiID, p.putLoID, p.putHiID} {
				pairsByInst[id] = append(pairsByInst[id], p)
			}
		}
		if *verbose {
			log.Printf("loaded %d instruments, %d box pairs (width=%d, tick=%s)",
				len(instruments), len(pairs), *widthInt, tick)
		}
	}

	// Phase 2: Stream CMBP1 and simulate legging.
	brk := newBroker(*rateLimit)
	book := newLedger(startingCash)
	quotes := make(map[uint32]*quote)
	var active []*activeBox
	doneTrading := false
	completedCount := 0
	canceledCount := 0
	totalCommission := decimal.Zero

	// Track completed boxes for the report.
	type boxResult struct {
		pair    *boxPair
		long    bool
		legs    [4]boxLeg
		filled  int             // 4 = complete
		debit   decimal.Decimal // net per-share outflow
		pnl     decimal.Decimal // per-share P&L at expiry
		startTS clocky.Time
	}
	var results []boxResult

	// Set up record source: file (backtest) or live stream.
	var nextRecord func() ([]byte, error)
	var cleanup func()

	if *live {
		apiKey := databento.MustLoadDefaultKey()
		client, err := databento.Dial(*dataset, apiKey)
		if err != nil {
			log.Fatalf("dial %s: %v", *dataset, err)
		}
		cleanup = func() { client.Close() }
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
			log.Fatalf("start session: %v", err)
		}
		log.Printf("streaming %s (dbn v%d)...", parentSym, meta.Version)
		nextRecord = func() ([]byte, error) { return client.NextRecord() }
	} else {
		f, err := os.Open(*dataPath)
		if err != nil {
			log.Fatalf("open data: %v", err)
		}
		cleanup = func() { f.Close() }
		meta, err := databento.DecodeMetadata(f)
		if err != nil {
			log.Fatalf("decode metadata: %v", err)
		}
		br := bufio.NewReaderSize(f, 2<<20)
		nextRecord = func() ([]byte, error) {
			rec, err := databento.DecodeRecord(br)
			if err != nil {
				return nil, err
			}
			return databento.UpgradeRecord(meta.Version, rec)
		}
	}
	defer cleanup()

	records = 0
	cmbp1Size := int(unsafe.Sizeof(databento.CMBP1{}))
	instSize := int(unsafe.Sizeof(databento.Instrument{}))

	expSeen := make(map[int]bool)
	var defsSeen int
	for {
		rec, err := nextRecord()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("decode record: %v", err)
		}
		records++

		if *live && records < 100 {
			rtype := databento.RType(rec[1])
			log.Printf("live: record[%d] rtype=%s len=%d", records, rtype, len(rec))
		}

		// Dispatch non-CMBP1 record types.
		if len(rec) >= 2 {
			rtype := databento.RType(rec[1])
			switch rtype {
			case databento.RTypeInstrumentDef:
				defsSeen++
				inst := (*databento.Instrument)(unsafe.Pointer(&rec[0]))
				expUTC := time.Unix(int64(inst.Expiration)/1e9, int64(inst.Expiration)%1e9).UTC()
				expDateInt := expUTC.Year()*10000 + int(expUTC.Month())*100 + expUTC.Day()
				if !expSeen[expDateInt] {
					log.Printf("live: saw expiration %d", expDateInt)
					expSeen[expDateInt] = true
				}
				if *verbose && defsSeen%100 == 1 {
					log.Printf("live: saw def %s (exp %d, want %d)", 
						inst.GetRawSymbol(), expDateInt, queryDateInt)
				}
				if liveDB != nil && len(rec) >= instSize {
					if liveDB.addInstrument(inst, queryDateInt) {
						// Rebuild instruments and pairs as definitions arrive.
						instruments = liveDB.instruments
						pairs = liveDB.buildPairs(*widthInt)
						pairsByInst = make(map[uint32][]*boxPair)
						for i := range pairs {
							p := &pairs[i]
							for _, id := range []uint32{p.callLoID, p.callHiID, p.putLoID, p.putHiID} {
								pairsByInst[id] = append(pairsByInst[id], p)
							}
						}
						if !liveDB.minTick.IsZero() {
							tick = liveDB.minTick
						}
						if *verbose && len(pairs) != lastPairCount {
							log.Printf("live: %d instruments, %d box pairs (width=%d, tick=%s)",
								len(instruments), len(pairs), *widthInt, tick)
							lastPairCount = len(pairs)
						}
					}
				}
				continue
			case databento.RTypeSymbolMapping:
				if liveDB != nil {
					data := databento.CastRecord(rec)
					var id uint32
					var osi string
					if m, ok := data.(*databento.SymbolMappingMsg); ok {
						id = m.Header.InstrumentID
						osi = m.GetSTypeOutSymbol()
					}
					if id != 0 && osi != "" {
						if *verbose && records < 500 {
							log.Printf("live: mapping id=%d osi=%q", id, osi)
						}
						_, _, exp, err := parseOSI(osi)
						if err == nil && !expSeen[exp] {
							log.Printf("live: saw expiration %d (mapping: %s)", exp, osi)
							expSeen[exp] = true
						}
						if liveDB.addInstrumentFromOSI(id, osi, queryDateInt) {
							instruments = liveDB.instruments
							pairs = liveDB.buildPairs(*widthInt)
							pairsByInst = make(map[uint32][]*boxPair)
							for i := range pairs {
								p := &pairs[i]
								for _, id := range []uint32{p.callLoID, p.callHiID, p.putLoID, p.putHiID} {
									pairsByInst[id] = append(pairsByInst[id], p)
								}
							}
							log.Printf("live: registered %s id=%d (%d instruments, %d box pairs)",
								osi, id, len(instruments), len(pairs))
							lastPairCount = len(pairs)
						}
					}
				}
				continue
			case databento.RTypeError:
				msg := databento.CastRecord(rec).(databento.ErrorMsg)
				log.Printf("gateway error: %s", msg.Err)
				continue
			case databento.RTypeSystem:
				msg := databento.CastRecord(rec).(databento.SystemMsg)
				if *verbose && !msg.IsHeartbeat() {
					log.Printf("gateway: %s", msg.Msg)
				}
				continue
			case databento.RTypeCMBP1:
				break
			default:
				if *verbose && records < 1000 {
					log.Printf("live: unknown rtype=0x%02x len=%d", rec[1], len(rec))
				}
				continue
			}
		}

		if len(rec) < cmbp1Size {
			continue
		}
		cmbp1Tick := (*databento.CMBP1)(unsafe.Pointer(&rec[0]))
		records++

		if *live && (records%100000 == 0 || clocky.Since(lastDefTS) > 10*clocky.Second) {
			log.Printf("live: records=%d instruments=%d pairs=%d active=%d (ts=%s)",
				records, len(instruments), len(pairs), len(active), formatET(cmbp1Tick.Header.TSEvent))
			lastDefTS = clocky.Now()
		}

		if *live {
			// Live mode: use real wall clock, not simulated.
		} else {
			clocky.SetNow(cmbp1Tick.TSRecv)
		}

		instID := cmbp1Tick.Header.InstrumentID
		info, ok := instruments[instID]
		if !ok {
			continue
		}

		if *live && records < 100000 && records%10000 == 0 {
			log.Printf("live: quote for %s id=%d bid=%s ask=%s",
				info.symbol, instID, dbnPrice(cmbp1Tick.Levels[0].BidPx).Format(2),
				dbnPrice(cmbp1Tick.Levels[0].AskPx).Format(2))
		}

		ts := cmbp1Tick.Header.TSEvent

		// Once done trading, only update quotes for instruments we hold
		// positions in (needed for settlement). Skip everything else.
		if doneTrading {
			if book.position[instID] == 0 {
				continue
			}
			q := quotes[instID]
			if q == nil {
				continue
			}
			bidPx := cmbp1Tick.Levels[0].BidPx
			askPx := cmbp1Tick.Levels[0].AskPx
			if bidPx != databento.UndefPrice {
				q.bidPx = dbnPrice(bidPx)
			}
			if askPx != databento.UndefPrice {
				q.askPx = dbnPrice(askPx)
			}
			continue
		}

		// Update quote. UndefPrice → zero (clears stale quotes).
		q, ok := quotes[instID]
		if !ok {
			q = &quote{}
			quotes[instID] = q
		}
		bidPx := cmbp1Tick.Levels[0].BidPx
		askPx := cmbp1Tick.Levels[0].AskPx
		if bidPx != databento.UndefPrice {
			q.bidPx = dbnPrice(bidPx)
			q.bidSz = cmbp1Tick.Levels[0].BidSz
			q.bidPb = cmbp1Tick.Levels[0].BidPb
		} else {
			q.bidPx = decimal.Zero
			q.bidSz = 0
		}
		if askPx != databento.UndefPrice {
			q.askPx = dbnPrice(askPx)
			q.askSz = cmbp1Tick.Levels[0].AskSz
			q.askPb = cmbp1Tick.Levels[0].AskPb
		} else {
			q.askPx = decimal.Zero
			q.askSz = 0
		}

		if ts.Before(startMTV) {
			continue
		}

		// --- Process fills on active boxes (adversarial midpoint model) ---
		// After latency, check if our limit has crossed the current midpoint.
		// Fill only when the NBBO moved against us (adversarial). If still
		// favorable, reprice with decayed greed and restart latency.
		// Greed decays linearly over patience and as legs fill, so early
		// legs capture the most spread and later legs accept worse prices.
		for _, ab := range active {
			elapsed := clocky.Duration(ts - ab.startTS)
			for i, ord := range ab.orders {
				if ord == nil || ord.instID != instID {
					continue
				}
				if q.bidPx.IsZero() || q.askPx.IsZero() || q.askPx.Cmp(q.bidPx) <= 0 {
					continue
				}

				// Apply pending modify once its latency has elapsed.
				if !ord.pendingPx.IsZero() && clocky.Duration(ts-ord.modifyTS) >= *latencyFlag {
					ord.limitPx = ord.pendingPx
					ord.postTime = ord.modifyTS
					ord.pendingPx = decimal.Zero
				}

				// New orders wait for initial latency before becoming live.
				if clocky.Duration(ts-ord.postTime) < *latencyFlag {
					continue
				}

				mid := q.bidPx.Add(q.askPx).DivInt(2)

				// Check if our limit has crossed the midpoint.
				// BUY: limit >= mid means we've crossed (bad for us).
				// SELL: limit <= mid means we've crossed (bad for us).
				var crossed bool
				var fillPx decimal.Decimal
				if ord.side == 'B' {
					if ord.limitPx.Cmp(mid) >= 0 {
						crossed = true
						fillPx = ord.limitPx.Min(snapAsk(q.askPx))
					}
				} else {
					if ord.limitPx.Cmp(mid) <= 0 {
						crossed = true
						fillPx = ord.limitPx.Max(snapBid(q.bidPx))
					}
				}

				if crossed {
					info := instruments[instID]
					ab.legs[i] = &boxLeg{
						instID:  instID,
						class:   info.class,
						strike:  info.strike,
						side:    ord.side,
						fillPx:  fillPx,
						fillTS:  ts,
						orderTS: ord.postTime,
					}
					ab.orders[i] = nil
					ab.filled++

					book.recordFill(ts, instID, ord.side, fillPx)
					book.payCommission(commPerLeg)
					totalCommission = totalCommission.Add(commPerLeg)

					if *verbose {
						side := "Buy "
						if ord.side == 'S' {
							side = "Sell"
						}
						fillDur := time.Duration(ts-ord.postTime) * time.Nanosecond
						var improv decimal.Decimal
						if ord.side == 'B' {
							improv = mid.Sub(fillPx)
						} else {
							improv = fillPx.Sub(mid)
						}
						greed := computeGreed(*greedFlag, elapsed, *patienceFlag, ab.filled)
						log.Printf("    %s %c%-5d %s @ %-8s  mid %s  improv %s  greed %.0f%%  (fill: %s)",
							side, info.class, info.strike.Int(),
							formatET(ts), fillPx.Format(2),
							mid.Format(2), formatPnl(improv),
							float64(greed.MulInt(100).Int64()),
							formatDuration(fillDur))
					}
					continue
				}

				// Not filled and no modify in flight: reprice with decayed
				// greed, clamped to budget. Skip if rate limited.
				if ord.pendingPx.IsZero() && brk.canCall(ts, 1) {
					greed := computeGreed(*greedFlag, elapsed, *patienceFlag, ab.filled)
					newPx := greedyMid(q, ord.side, greed)
					budget := budgetLimit(ab, i, width, minEdge, quotes)
					newPx = clampLimit(ord.side, newPx, budget)
					if !newPx.IsZero() && !newPx.IsNegative() && newPx.Cmp(ord.limitPx) != 0 {
						brk.modifyOrder(ord, newPx, ts)
					}
				}
			}
		}

		// --- Harvest completed boxes ---
		j := 0
		for _, ab := range active {
			if ab.filled == 4 {
				// Compute per-share P&L.
				var netOutflow decimal.Decimal
				var br boxResult
				br.pair = ab.pair
				br.long = ab.long
				br.filled = 4
				br.startTS = ab.startTS
				for i, leg := range ab.legs {
					br.legs[i] = *leg
					if leg.side == 'B' {
						netOutflow = netOutflow.Add(leg.fillPx)
					} else {
						netOutflow = netOutflow.Sub(leg.fillPx)
					}
				}
				br.debit = netOutflow
				if ab.long {
					br.pnl = width.Sub(netOutflow)
				} else {
					br.pnl = netOutflow.Neg().Sub(width)
				}
				results = append(results, br)
				completedCount++
				if *verbose {
					dir := "LONG"
					if !ab.long {
						dir = "SHORT"
					}
					log.Printf("  ** completed %s %d/%d  debit: $%s  pnl: %s/share  cash: $%s",
						dir, ab.pair.loStrike.Int(), ab.pair.hiStrike.Int(),
						br.debit.Format(2), formatPnl(br.pnl), book.cash.Format(2))
				}
				continue
			}
			active[j] = ab
			j++
		}
		active = active[:j]

		// --- Past expiry: settle everything ---
		if !ts.Before(expiryMTV) {
			break
		}

		// --- Past cutoff: don't open new boxes ---
		if !ts.Before(cutoffMTV) {
			if len(active) == 0 {
				doneTrading = true
			}
			continue
		}

		// --- Scan for new box opportunities ---
		if len(active) >= *maxOpen {
			continue
		}
		if !brk.canCall(ts, 4) { // startBox places 4 limit orders
			continue
		}

		// Collect all qualifying candidates, pick the one with the best edge.
		var bestPair *boxPair
		var bestLong bool
		var bestEdge decimal.Decimal
		var bestDebit decimal.Decimal
		for _, bp := range pairsByInst[instID] {
			if hasActivePair(active, bp) {
				continue
			}

			qCL := quotes[bp.callLoID]
			qCH := quotes[bp.callHiID]
			qPL := quotes[bp.putLoID]
			qPH := quotes[bp.putHiID]
			if qCL == nil || qCH == nil || qPL == nil || qPH == nil {
				continue
			}
			if !legQualified(qCL, minBid, maxSpread) ||
				!legQualified(qCH, minBid, maxSpread) ||
				!legQualified(qPL, minBid, maxSpread) ||
				!legQualified(qPH, minBid, maxSpread) {
				continue
			}

			// Long box: provide liquidity → buy at bids, sell at asks.
			longDebit := snapBid(qCL.bidPx).Add(snapBid(qPH.bidPx)).
				Sub(snapAsk(qCH.askPx)).Sub(snapAsk(qPL.askPx))
			longEdge := width.Sub(longDebit)

			// Short box: provide liquidity → sell at asks, buy at bids.
			shortCredit := snapAsk(qCL.askPx).Add(snapAsk(qPH.askPx)).
				Sub(snapBid(qCH.bidPx)).Sub(snapBid(qPL.bidPx))
			shortEdge := shortCredit.Sub(width)

			// Pick the better direction for this pair.
			var edge decimal.Decimal
			var long bool
			var debit decimal.Decimal
			if longEdge.Cmp(shortEdge) >= 0 {
				edge = longEdge
				long = true
				debit = longDebit
			} else {
				edge = shortEdge
				long = false
				debit = shortCredit.Neg() // store as negative (credit)
			}
			if edge.Cmp(minEdge) < 0 || edge.Cmp(maxEdge) > 0 {
				continue
			}
			if bestPair == nil || edge.Cmp(bestEdge) > 0 {
				bestPair = bp
				bestLong = long
				bestEdge = edge
				bestDebit = debit
			}
		}
		if bestPair != nil && len(active) < *maxOpen {
			ab := startBox(brk, bestPair, bestLong, quotes, ts, *greedFlag, width, minEdge)
			active = append(active, ab)
			if *verbose {
				dir := "LONG"
				if !bestLong {
					dir = "SHORT"
				}
				log.Printf("  box #%d %s %d/%d  est.debit: $%s  est.edge: %s/share",
					len(results)+len(active), dir,
					bestPair.loStrike.Int(), bestPair.hiStrike.Int(),
					bestDebit.Format(2), formatPnl(bestEdge))
				printLegQuotes(bestPair, quotes)
			}
		}
	}

	// --- Expiry settlement ---
	// Settle all open positions at intrinsic value.
	// For options: intrinsic = max(S-K, 0) for calls, max(K-S, 0) for puts.
	// We don't know S directly, but for a completed box all legs cancel out
	// to exactly `width` at any S. For incomplete boxes, we settle each
	// individual position at the last mid price (best estimate of intrinsic
	// near expiry).
	//
	// Record incomplete boxes first.
	for _, ab := range active {
		if ab.filled == 0 {
			cancelBox(brk, ab)
			canceledCount++
			continue
		}
		// Incomplete box: record what we have.
		var br boxResult
		br.pair = ab.pair
		br.long = ab.long
		br.filled = ab.filled
		br.startTS = ab.startTS
		var netOutflow decimal.Decimal
		for i, leg := range ab.legs {
			if leg != nil {
				br.legs[i] = *leg
				if leg.side == 'B' {
					netOutflow = netOutflow.Add(leg.fillPx)
				} else {
					netOutflow = netOutflow.Sub(leg.fillPx)
				}
			} else if ab.orders[i] != nil {
				ord := ab.orders[i]
				info := instruments[ord.instID]
				br.legs[i] = boxLeg{
					instID:  ord.instID,
					class:   info.class,
					strike:  info.strike,
					side:    ord.side,
					fillPx:  ord.limitPx, // unfilled: store limit for display
					orderTS: ord.postTime,
				}
			}
		}
		br.debit = netOutflow
		results = append(results, br)
		cancelBox(brk, ab)
	}

	// Settle all positions in the ledger at last mid.
	var settlementCash decimal.Decimal
	for instID, pos := range book.position {
		if pos == 0 {
			continue
		}
		q := quotes[instID]
		var midPx decimal.Decimal
		if q != nil && !q.bidPx.IsZero() && !q.askPx.IsZero() {
			midPx = q.bidPx.Add(q.askPx).DivInt(2)
		} else if q != nil && !q.bidPx.IsZero() {
			midPx = q.bidPx
		} else if q != nil && !q.askPx.IsZero() {
			midPx = q.askPx
		}
		cf := book.settle(instID, midPx)
		settlementCash = settlementCash.Add(cf)
		if *verbose && !cf.IsZero() {
			info := instruments[instID]
			log.Printf("  settle %c%-5d pos=%d @ mid $%s  cash: %s",
				info.class, info.strike.Int(), pos,
				midPx.Format(2), formatPnl(cf))
		}
	}

	// --- Print results ---
	log.Printf("\n%s 0DTE box spread legging", *dateStr)
	log.Printf("  parameters: width=%d  tick=%s  mult=%d  edge=%s  maxspread=%s  minbid=%s  maxopen=%d  latency=%s  greed=%s  patience=%s",
		*widthInt, tick, multiplier, minEdge, maxSpread, minBid, *maxOpen, *latencyFlag, *greedFlag, *patienceFlag)
	log.Printf("  commission: $%s/leg ($%s/box, $%s/share/box)",
		commPerLeg.Format(2), commPerBox.Format(2), commPerSharePerBox.Format(4))
	log.Printf("  starting cash: $%s", startingCash.Format(2))
	log.Println()

	var totalPnl decimal.Decimal
	var totalFillTime time.Duration
	var totalLegs int
	var longCount, shortCount int
	var incompleteCount int

	for i, br := range results {
		dir := "LONG"
		if !br.long {
			dir = "SHORT"
		}
		complete := br.filled == 4
		label := fmt.Sprintf("box #%d", i+1)
		if !complete {
			label = fmt.Sprintf("box #%d (%d/4)", i+1, br.filled)
			incompleteCount++
		}
		log.Printf("  %s %s %d/%d", label, dir,
			br.pair.loStrike.Int(), br.pair.hiStrike.Int())
		for _, leg := range br.legs {
			if leg.instID == 0 {
				continue
			}
			side := "Buy "
			if leg.side == 'S' {
				side = "Sell"
			}
			if leg.fillTS == 0 {
				// Unfilled leg.
				log.Printf("    %s %c%-5d (unfilled, limit was $%s)",
					side, leg.class, leg.strike.Int(), leg.fillPx.Format(2))
			} else {
				fillDur := time.Duration(leg.fillTS-leg.orderTS) * time.Nanosecond
				log.Printf("    %s %c%-5d %s @ $%-8s (fill: %s)",
					side, leg.class, leg.strike.Int(),
					formatET(leg.fillTS), leg.fillPx.Format(2),
					formatDuration(fillDur))
				totalLegs++
				totalFillTime += fillDur
			}
		}
		if complete {
			log.Printf("    debit: $%s  profit: %s/share (%s/contract)",
				br.debit.Format(2), formatPnl(br.pnl),
				formatPnl(br.pnl.MulInt(multiplier)))
			if br.long {
				longCount++
			} else {
				shortCount++
			}
			totalPnl = totalPnl.Add(br.pnl)
		} else {
			log.Printf("    filled legs debit: $%s  (settled in ledger)",
				br.debit.Format(2))
		}
	}

	// --- Reconciliation ---
	log.Println()
	log.Println("  --- holdings at expiry ---")
	openPositions := 0
	for instID, pos := range book.position {
		if pos == 0 {
			continue
		}
		openPositions++
		info := instruments[instID]
		q := quotes[instID]
		var midStr string
		if q != nil && !q.bidPx.IsZero() {
			midStr = q.bidPx.Add(q.askPx).DivInt(2).Format(2)
		} else {
			midStr = "?"
		}
		log.Printf("  %c%-5d  pos: %+d  cost: $%s  mid: $%s",
			info.class, info.strike.Int(), pos,
			book.cost[instID].Format(2), midStr)
	}
	if openPositions == 0 {
		log.Println("  (no open positions)")
	}

	log.Println()
	log.Println("  --- trade blotter ---")
	log.Printf("  total fills: %d", len(book.fills))
	log.Printf("  total commission: $%s", totalCommission.Format(2))
	if !settlementCash.IsZero() {
		log.Printf("  settlement cash flow: %s", formatPnl(settlementCash))
	}

	log.Println()
	log.Println("  --- summary ---")
	log.Printf("  completed boxes: %d (%d long, %d short)",
		completedCount, longCount, shortCount)
	if incompleteCount > 0 {
		log.Printf("  incomplete boxes: %d", incompleteCount)
	}
	if canceledCount > 0 {
		log.Printf("  canceled (0-fill): %d", canceledCount)
	}
	if totalLegs > 0 {
		avgFill := totalFillTime / time.Duration(totalLegs)
		log.Printf("  avg fill time: %s per leg", formatDuration(avgFill))
	}
	log.Printf("  completed P&L: %s/share (%s/contract)",
		formatPnl(totalPnl), formatPnl(totalPnl.MulInt(multiplier)))
	log.Printf("  ending cash (ledger): $%s", book.cash.Format(2))
	log.Printf("  records: %d", records)
}

// -----------------------------------------------------------------------
// Definitions loader
// -----------------------------------------------------------------------

type defBuilder struct {
	instruments map[uint32]optionInfo
	strikeMap   map[decimal.Decimal]*strikeLevel
	minTick     decimal.Decimal
}

func (db *defBuilder) addInstrumentFromOSI(id uint32, osi string, queryDateInt int) bool {
	strike, class, expDateInt, err := parseOSI(osi)
	if err != nil {
		return false
	}
	if expDateInt != queryDateInt {
		return false
	}
	db.instruments[id] = optionInfo{
		strike: strike,
		class:  class,
		symbol: osi,
	}
	sl, ok := db.strikeMap[strike]
	if !ok {
		sl = &strikeLevel{strike: strike}
		db.strikeMap[strike] = sl
	}
	if class == 'C' {
		sl.callID = id
	} else {
		sl.putID = id
	}
	return true
}

func newDefBuilder() *defBuilder {
	return &defBuilder{
		instruments: make(map[uint32]optionInfo),
		strikeMap:   make(map[decimal.Decimal]*strikeLevel),
	}
}

// addInstrument processes a single instrument definition. Returns true if
// it was a 0DTE option matching queryDateInt.
func (db *defBuilder) addInstrument(inst *databento.Instrument, queryDateInt int) bool {
	var class byte
	switch inst.InstrumentClass {
	case databento.InstrumentClassCall:
		class = 'C'
	case databento.InstrumentClassPut:
		class = 'P'
	default:
		return false
	}

	expUTC := time.Unix(int64(inst.Expiration)/1e9, int64(inst.Expiration)%1e9).UTC()
	expDateInt := expUTC.Year()*10000 + int(expUTC.Month())*100 + expUTC.Day()

	// databento stores expiration as UTC midnight of the expiry day.
	// queryDateInt is also YYYYMMDD in UTC/NY terms for this program.
	if expDateInt != queryDateInt {
		if clocky.Since(lastDefTS) > 10*clocky.Second {
			log.Printf("debug: ignoring %s (exp %d, want %d)", inst.GetRawSymbol(), expDateInt, queryDateInt)
			lastDefTS = clocky.Now()
		}
		return false
	}

	if inst.StrikePrice == databento.UndefPrice {
		return false
	}

	if inst.MinPriceIncrement != 0 && inst.MinPriceIncrement != databento.UndefPrice {
		t := dbnPrice(inst.MinPriceIncrement)
		if db.minTick.IsZero() || t.Cmp(db.minTick) < 0 {
			db.minTick = t
		}
	}

	strike := dbnPrice(inst.StrikePrice)
	id := inst.Header.InstrumentID
	db.instruments[id] = optionInfo{
		strike: strike,
		class:  class,
		symbol: inst.GetRawSymbol(),
	}

	sl, ok := db.strikeMap[strike]
	if !ok {
		sl = &strikeLevel{strike: strike}
		db.strikeMap[strike] = sl
	}
	if class == 'C' {
		sl.callID = id
	} else {
		sl.putID = id
	}
	return true
}

// buildPairs constructs box pairs from accumulated strikes.
func (db *defBuilder) buildPairs(widthInt int) []boxPair {
	strikes := make([]strikeLevel, 0, len(db.strikeMap))
	for _, sl := range db.strikeMap {
		if sl.callID != 0 && sl.putID != 0 {
			strikes = append(strikes, *sl)
		}
	}
	sort.Slice(strikes, func(i, j int) bool {
		return strikes[i].strike.Cmp(strikes[j].strike) < 0
	})

	widthDec := decimal.FromInt(widthInt)
	var pairs []boxPair
	for i, lo := range strikes {
		target := lo.strike.Add(widthDec)
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
	return pairs
}

// loadDefinitions reads instrument definitions from a DBN file.
func loadDefinitions(path string, queryDateInt, widthInt int) (map[uint32]optionInfo, []boxPair, decimal.Decimal, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, decimal.Zero, err
	}
	defer f.Close()

	meta, err := databento.DecodeMetadata(f)
	if err != nil {
		return nil, nil, decimal.Zero, fmt.Errorf("decode metadata: %w", err)
	}

	db := newDefBuilder()
	instSize := int(unsafe.Sizeof(databento.Instrument{}))

	for {
		rec, err := databento.DecodeRecord(f)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, decimal.Zero, fmt.Errorf("decode record: %w", err)
		}
		rec, err = databento.UpgradeRecord(meta.Version, rec)
		if err != nil {
			return nil, nil, decimal.Zero, fmt.Errorf("upgrade record: %w", err)
		}
		if len(rec) < instSize {
			continue
		}
		inst := (*databento.Instrument)(unsafe.Pointer(&rec[0]))
		db.addInstrument(inst, queryDateInt)
	}

	pairs := db.buildPairs(widthInt)
	return db.instruments, pairs, db.minTick, nil
}

// parseOSI parses an OSI symbol like "SPY   261218C00710000".
func parseOSI(osi string) (strike decimal.Decimal, class byte, expDateInt int, err error) {
	if len(osi) < 21 {
		return decimal.Zero, 0, 0, fmt.Errorf("osi too short: %q", osi)
	}
	// Strike is at the end: 13-21 (8 chars)
	strikeStr := osi[13:21]
	// 5 digits + 3 decimals: 00710000 -> 710.000
	strikeVal, err := strconv.ParseInt(strikeStr, 10, 64)
	if err != nil {
		return decimal.Zero, 0, 0, fmt.Errorf("bad strike %q: %w", strikeStr, err)
	}
	strike = decimal.FromInt(int(strikeVal)).DivInt(1000)
	class = osi[12] // 'C' or 'P'
	// Expiry YYMMDD at 6-12
	expStr := osi[6:12]
	var expVal int
	expVal, err = strconv.Atoi("20" + expStr) // assume 20xx
	if err != nil {
		return decimal.Zero, 0, 0, fmt.Errorf("bad expiry %q: %w", expStr, err)
	}
	return strike, class, expVal, nil
}

