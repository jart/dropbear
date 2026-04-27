// study replays recorded sip data to understand the frequency
// of the quote condition codes that come through the SIP feed
//
// usage: go test -run Study -timeout 0 -- ~/sip/sipdata-2026-01-07.sip
package main

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
	"dropbear/symbol"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

func init() {
	netty.SetOffline()
}

var (
	flagThreshold = decimal.Flag("threshold", "0.3", "imbalance ratio threshold (0-1)")
	flagISO       = decimal.Flag("iso", "200", "net ISO shares threshold")
	flagGreed     = decimal.FlagBPS("greed", "0", "basis points profit target over cost")
	flagSize      = decimal.Flag("size", "1000", "capital per trade in usd")
	flagFloor     = decimal.Flag("floor", "2", "minimum trade quantity in shares (2+ ensures negative fees)")
	flagPatience  = clocky.DurationFlag("patience", "5m", "cancel unfilled orders after this")
	flagDayOnly   = flag.Bool("dayonly", false, "restrict analysis to day session only")
)

// fee constants (same as cmd/trader/fee.go)
var (
	kTakerFeePerShare  = decimal.Parse("0.0020")
	kMakerFeePerShare  = decimal.Parse("-0.0018")
	kSECFeePerMillion  = decimal.Parse("20.60")
	kTAFFeePerShare    = decimal.Parse("0.000195")
	kCATFeePerTrade    = decimal.Parse("0.0003")
	kBrokerFeePerTrade = decimal.Parse("0.0025")
	kStdMarginRate     = decimal.Parse("0.3")
)

// horizons for measuring forward midpoint prediction
var horizons = []clocky.Duration{
	100 * clocky.Millisecond,
	500 * clocky.Millisecond,
	1 * clocky.Second,
	5 * clocky.Second,
	30 * clocky.Second,
	60 * clocky.Second,
}

const maxPendingSignals = 32

// price buckets for analysis (upper bound in dollars)
var priceBuckets = []decimal.Decimal{
	decimal.Parse("1"),
	decimal.Parse("5"),
	decimal.Parse("10"),
	decimal.Parse("25"),
	decimal.Parse("50"),
	decimal.Parse("100"),
	decimal.Parse("250"),
	decimal.Parse("500"),
	decimal.Parse("99999"),
}

// spread buckets for analysis (upper bound in ticks)
var spreadBuckets = []int{1, 2, 3, 5, 10, 20, 50, 100, 9999}

// per-tick spread values for entry offset analysis
// individual ticks 2-10 then catch-all for 11+
var entrySpreadBuckets = []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 9999}

type symState struct {
	quote    sip.Quote
	hasQuote bool

	tradeConditionCode    map[string]uint64
	invalidTradeCondition uint64
	invalidTradeMidpoint  uint64

	quoteConditionCode map[string]uint64
	invalidQuotePrice  uint64
	invalidQuoteState  uint64

	isoNetFlow decimal.Decimal
	lastDecay  clocky.Time

	pending []pendingSignal

	position   decimal.Decimal
	costBasis  decimal.Decimal
	entryTime  clocky.Time
	orderSide  int8
	orderPrice decimal.Decimal
	orderTime  clocky.Time
}

type pendingSignal struct {
	time       clocky.Time
	direction  int8
	midpoint   decimal.Decimal
	bidPrice   decimal.Decimal
	askPrice   decimal.Decimal
	priceBkt   int
	spreadBkt  int
	entryBkt   int // index into entrySpreadBuckets
	spreadTick int // spread in ticks at signal time
	resolved   uint8
	minAsk     decimal.Decimal // lowest ask seen (buy signals)
	maxBid     decimal.Decimal // highest bid seen (sell signals)
}

type horizonStats struct {
	count      int64
	correct    int64
	moveSum    int64 // bps * 1000 in predicted direction
	absMoveSum int64
}

type bucketStats struct {
	signals int64
	hstats  [6]horizonStats
}

type entryStats struct {
	signals int64
	fills   [3]int64 // fills for entry at bid+1, bid+2, bid+3 ticks
}

type symInventory struct {
	name   string
	quotes int64
	trades int64
}

var (
	gSymbols  map[symbol.Symbol]*symState
	gSymQuote map[symbol.Symbol]int64
	gSymTrade map[symbol.Symbol]int64

	gTotalQuotes int64
	gTotalTrades int64
	gTotalOther  int64
	gFirstTime   clocky.Time
	gLastTime    clocky.Time

	gTotalSignals int64
	gBuySignals   int64
	gSellSignals  int64
	gHStats       [6]horizonStats

	// per-bucket analysis
	gPriceBuckets  []bucketStats
	gSpreadBuckets []bucketStats
	gEntryStats    []entryStats

	// round-trip simulation
	gSimRoundTrips int64
	gSimWins       int64
	gSimPnL        decimal.Decimal
	gSimFees       decimal.Decimal
	gSimTimeouts   int64
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: go test -run Study -timeout 0 -- <sipfile>...\n")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		study(path)
	}
}

func study(path string) {
	f, err := sip.OpenFile(path)
	if err != nil {
		log.Fatalf("%s: %v", path, err)
	}
	defer f.Close()

	gSymbols = make(map[symbol.Symbol]*symState, 16384)
	gSymQuote = make(map[symbol.Symbol]int64, 16384)
	gSymTrade = make(map[symbol.Symbol]int64, 16384)
	gTotalQuotes = 0
	gTotalTrades = 0
	gTotalOther = 0
	gTotalSignals = 0
	gBuySignals = 0
	gSellSignals = 0
	gHStats = [6]horizonStats{}

	gPriceBuckets = make([]bucketStats, len(priceBuckets))
	gSpreadBuckets = make([]bucketStats, len(spreadBuckets))
	gEntryStats = make([]entryStats, len(entrySpreadBuckets))
	gSimRoundTrips = 0
	gSimWins = 0
	gSimPnL = decimal.Zero
	gSimFees = decimal.Zero
	gSimTimeouts = 0

	count := f.Count()
	gFirstTime = f.Get(0).Timestamp
	gLastTime = f.Get(count - 1).Timestamp

	pctStep := count / 100
	if pctStep == 0 {
		pctStep = 1
	}

	for i := 0; i < count; i++ {
		if i%pctStep == 0 {
			fmt.Fprintf(os.Stderr, "\r%d%%", i*100/count)
		}
		msg := f.Get(i)
		sym := msg.Symbol
		switch msg.Type {
		case sip.MessageTypeQuote:
			gTotalQuotes++
			gSymQuote[sym]++
			processQuote(sym, msg.Quote())
		case sip.MessageTypeTrade:
			gTotalTrades++
			gSymTrade[sym]++
			processTrade(sym, msg.Trade())
		default:
			gTotalOther++
		}
	}
	fmt.Fprintf(os.Stderr, "\r     \r")

	printResults(path, count)
}

func getOrCreate(sym symbol.Symbol) *symState {
	st := gSymbols[sym]
	if st == nil {
		st = &symState{}
		gSymbols[sym] = st
	}
	return st
}

func priceBucket(mid decimal.Decimal) int {
	for i, bound := range priceBuckets {
		if mid.Cmp(bound) < 0 {
			return i
		}
	}
	return len(priceBuckets) - 1
}

func spreadBucket(ticks int) int {
	for i, bound := range spreadBuckets {
		if ticks <= bound {
			return i
		}
	}
	return len(spreadBuckets) - 1
}

func entrySpreadBucket(ticks int) int {
	for i, bound := range entrySpreadBuckets {
		if ticks <= bound {
			return i
		}
	}
	return len(entrySpreadBuckets) - 1
}

func processQuote(sym symbol.Symbol, q *sip.Quote) {
	st := getOrCreate(sym)
	quoteCondition := q.Conditions.String()
	if st.quoteConditionCode == nil {
		// todo(gsalaz98): real shitty for now but need to initialize map
		st.quoteConditionCode = make(map[string]uint64, 0)
	}

	_, ok := st.quoteConditionCode[quoteCondition]
	if !ok {
		st.quoteConditionCode[quoteCondition] = 0
	}

	st.quoteConditionCode[quoteCondition]++
	if q.BidPrice.IsZero() || q.AskPrice.IsZero() {
		st.invalidQuotePrice++
		return
	}
	if q.BidPrice.Cmp(q.AskPrice) >= 0 {
		st.invalidQuoteState++
		return
	}

	now := q.Timestamp

	if st.lastDecay != 0 {
		elapsed := now.Sub(st.lastDecay)
		if elapsed >= clocky.Second {
			n := int(elapsed / clocky.Second)
			if n > 20 {
				st.isoNetFlow = decimal.Zero
			} else {
				for j := 0; j < n && !st.isoNetFlow.IsZero(); j++ {
					st.isoNetFlow = st.isoNetFlow.Half()
				}
			}
			st.lastDecay = now
		}
	} else {
		st.lastDecay = now
	}

	mid := q.Midpoint()

	// resolve pending signals
	for j := len(st.pending) - 1; j >= 0; j-- {
		sig := &st.pending[j]
		elapsed := now.Sub(sig.time)

		// track best fill price seen so far
		if sig.direction == 1 {
			if sig.minAsk.IsZero() || q.AskPrice.Cmp(sig.minAsk) < 0 {
				sig.minAsk = q.AskPrice
			}
		} else {
			if sig.maxBid.IsZero() || q.BidPrice.Cmp(sig.maxBid) > 0 {
				sig.maxBid = q.BidPrice
			}
		}

		allDone := true
		for h, hz := range horizons {
			bit := uint8(1 << h)
			if sig.resolved&bit != 0 {
				continue
			}
			if elapsed >= hz {
				sig.resolved |= bit
				if sig.midpoint.IsZero() {
					continue
				}
				move := mid.Sub(sig.midpoint)
				bps1k := move.MulInt(10_000_000).Div(sig.midpoint).Truncate().Int64()
				if sig.direction < 0 {
					bps1k = -bps1k
				}
				record := func(hs *horizonStats) {
					hs.count++
					if bps1k > 0 {
						hs.correct++
					}
					hs.moveSum += bps1k
					if bps1k < 0 {
						hs.absMoveSum -= bps1k
					} else {
						hs.absMoveSum += bps1k
					}
				}
				record(&gHStats[h])
				record(&gPriceBuckets[sig.priceBkt].hstats[h])
				record(&gSpreadBuckets[sig.spreadBkt].hstats[h])

				// at last horizon, record entry fill analysis
				if h == len(horizons)-1 {
					recordEntryFills(sig)
				}
			} else {
				allDone = false
			}
		}
		if allDone {
			st.pending[j] = st.pending[len(st.pending)-1]
			st.pending = st.pending[:len(st.pending)-1]
		}
	}

	// check simulated order fills and timeouts
	if st.orderSide != 0 {
		filled := false
		if st.orderSide == 1 && q.AskPrice.Cmp(st.orderPrice) <= 0 {
			filled = true
		}
		if st.orderSide == -1 && q.BidPrice.Cmp(st.orderPrice) >= 0 {
			filled = true
		}
		if filled {
			onSimFill(st, q)
		} else if now.Sub(st.orderTime) > *flagPatience {
			if !st.position.IsZero() {
				forceExit(st, mid)
			}
			st.orderSide = 0
			gSimTimeouts++
		}
	}

	if q.Indicative() {
		st.quote = *q
		st.hasQuote = true
		return
	}

	if st.hasQuote && st.orderSide == 0 {
		if st.position.IsZero() {
			evaluateEntry(st, q)
		} else {
			placeExit(st, q)
		}
	}

	st.quote = *q
	st.hasQuote = true
}

func processTrade(sym symbol.Symbol, t *sip.Trade) {
	st := gSymbols[sym]
	if st == nil || !st.hasQuote {
		return
	}

	tConditions := strings.Split(t.Conditions.String(), ",")
	if st.tradeConditionCode == nil {
		st.tradeConditionCode = make(map[string]uint64, 0)
	}

	for _, tCondition := range tConditions {
		_, ok := st.tradeConditionCode[tCondition]
		if !ok {
			st.tradeConditionCode[tCondition] = 0
		}
		st.tradeConditionCode[tCondition]++
	}

	if !t.Conditions.Has(sip.TradeCondISO) {
		st.invalidTradeCondition++
		return
	}

	mid := st.quote.Midpoint()
	if mid.IsZero() {
		st.invalidTradeMidpoint++
		return
	}
	size := decimal.FromInt64(t.Size)
	cmp := t.Price.Cmp(mid)
	if cmp > 0 {
		st.isoNetFlow = st.isoNetFlow.Add(size)
	} else if cmp < 0 {
		st.isoNetFlow = st.isoNetFlow.Sub(size)
	}
}

func evaluateEntry(st *symState, q *sip.Quote) {
	if q.DangerousAsk() || q.DangerousBid() {
		return
	}
	if *flagDayOnly && cboe.GetSession(q.Timestamp) != cboe.SessionDay {
		return
	}

	mid := q.Midpoint()
	if mid.IsZero() {
		return
	}
	spread := q.AskPrice.Sub(q.BidPrice)
	tick := Tick(q.BidPrice)
	if tick.IsZero() {
		return
	}
	spreadTicks := spread.Div(tick).Truncate().Int()

	bidSize := decimal.FromInt(int(q.BidSize))
	askSize := decimal.FromInt(int(q.AskSize))
	totalSize := bidSize.Add(askSize)
	if totalSize.IsZero() {
		return
	}

	imbalance := bidSize.Sub(askSize).Div(totalSize)
	wantBuy := imbalance.Cmp(*flagThreshold) > 0 ||
		st.isoNetFlow.Cmp(*flagISO) > 0
	wantSell := imbalance.Cmp(flagThreshold.Neg()) < 0 ||
		st.isoNetFlow.Cmp(flagISO.Neg()) < 0

	if !wantBuy && !wantSell {
		return
	}

	// classify signal
	var dir int8
	if wantBuy {
		dir = 1
		gBuySignals++
	} else {
		dir = -1
		gSellSignals++
	}
	gTotalSignals++

	pbkt := priceBucket(mid)
	sbkt := spreadBucket(spreadTicks)
	gPriceBuckets[pbkt].signals++
	gSpreadBuckets[sbkt].signals++

	// track signal for horizon analysis
	if len(st.pending) < maxPendingSignals {
		st.pending = append(st.pending, pendingSignal{
			time:       q.Timestamp,
			direction:  dir,
			midpoint:   mid,
			bidPrice:   q.BidPrice,
			askPrice:   q.AskPrice,
			priceBkt:   pbkt,
			spreadBkt:  sbkt,
			entryBkt:   entrySpreadBucket(spreadTicks),
			spreadTick: spreadTicks,
		})
	}

	// only simulate trades where qty sizing produces a real order
	qty := tradeQuantity(mid, totalSize)
	if qty.IsZero() {
		return
	}

	if wantBuy {
		price := q.BidPrice.Add(tick)
		price = price.Min(q.AskPrice.Sub(tick))
		st.orderSide = 1
		st.orderPrice = price
		st.orderTime = q.Timestamp
	} else {
		price := q.AskPrice.Sub(tick)
		price = price.Max(q.BidPrice.Add(tick))
		st.orderSide = -1
		st.orderPrice = price
		st.orderTime = q.Timestamp
	}
}

func placeExit(st *symState, q *sip.Quote) {
	if q.DangerousAsk() || q.DangerousBid() {
		return
	}
	if st.position.IsPositive() {
		price := q.AskPrice
		if !flagGreed.IsZero() && !st.costBasis.IsZero() {
			minPrice := st.costBasis.Div(st.position).Mul(decimal.One.Add(*flagGreed))
			price = price.Max(minPrice)
			price = quantizeSellPrice(price)
		}
		st.orderSide = -1
		st.orderPrice = price
		st.orderTime = q.Timestamp
	} else if st.position.IsNegative() {
		price := q.BidPrice
		if !flagGreed.IsZero() && !st.costBasis.IsZero() {
			maxPrice := st.costBasis.Div(st.position).Mul(decimal.One.Sub(*flagGreed))
			price = price.Min(maxPrice)
			price = quantizeBuyPrice(price)
		}
		st.orderSide = 1
		st.orderPrice = price
		st.orderTime = q.Timestamp
	}
}

func onSimFill(st *symState, q *sip.Quote) {
	price := st.orderPrice
	mid := q.Midpoint()

	if st.position.IsZero() {
		qty := tradeQuantity(mid, decimal.FromInt(1000))
		if qty.IsZero() {
			st.orderSide = 0
			return
		}
		if st.orderSide == 1 {
			st.position = qty
			st.costBasis = price.Mul(qty)
		} else {
			st.position = qty.Neg()
			st.costBasis = price.Mul(qty).Neg()
		}
		st.entryTime = q.Timestamp
		fee := estimateFee(st.orderSide, qty, price, true)
		gSimFees = gSimFees.Add(fee)
		st.orderSide = 0
	} else {
		qty := st.position.Abs()
		var pnl decimal.Decimal
		if st.position.IsPositive() {
			avgCost := st.costBasis.Div(st.position)
			pnl = price.Sub(avgCost).Mul(qty)
		} else {
			avgCost := st.costBasis.Div(st.position)
			pnl = avgCost.Sub(price).Mul(qty)
		}
		gSimPnL = gSimPnL.Add(pnl)
		gSimRoundTrips++
		if pnl.IsPositive() {
			gSimWins++
		}
		var side int8
		if st.position.IsPositive() {
			side = -1
		} else {
			side = 1
		}
		fee := estimateFee(side, qty, price, true)
		gSimFees = gSimFees.Add(fee)
		st.position = decimal.Zero
		st.costBasis = decimal.Zero
		st.orderSide = 0
	}
}

func forceExit(st *symState, mid decimal.Decimal) {
	if st.position.IsZero() {
		return
	}
	qty := st.position.Abs()
	var pnl decimal.Decimal
	if st.position.IsPositive() {
		avgCost := st.costBasis.Div(st.position)
		pnl = mid.Sub(avgCost).Mul(qty)
	} else {
		avgCost := st.costBasis.Div(st.position)
		pnl = avgCost.Sub(mid).Mul(qty)
	}
	gSimPnL = gSimPnL.Add(pnl)
	gSimRoundTrips++
	if pnl.IsPositive() {
		gSimWins++
	}
	var side int8
	if st.position.IsPositive() {
		side = -1
	} else {
		side = 1
	}
	fee := estimateFee(side, qty, mid, true)
	gSimFees = gSimFees.Add(fee)
	st.position = decimal.Zero
	st.costBasis = decimal.Zero
}

func tradeQuantity(price, maxQty decimal.Decimal) decimal.Decimal {
	// same logic as cmd/trader TradeQuantity but without margin data
	// margin scaling would reduce qty for risky stocks; we assume standard 0.3
	qty := flagSize.Div(price)
	qty = qty.Truncate()
	lot := cboe.LotSize(price)
	if qty.Cmp(lot) >= 0 {
		qty = qty.QuantizeTruncate(lot)
	}
	qty = qty.Min(maxQty)
	if qty.Cmp(*flagFloor) < 0 {
		return decimal.Zero
	}
	return qty
}

func estimateFee(side int8, qty, price decimal.Decimal, firstFill bool) decimal.Decimal {
	fee := decimal.Zero
	if firstFill {
		fee = fee.Add(kCATFeePerTrade)
		fee = fee.Add(kBrokerFeePerTrade)
	}
	if side == -1 {
		fee = fee.Add(kTAFFeePerShare.Mul(qty))
		fee = fee.Add(price.Mul(qty).Mul(kSECFeePerMillion).DivInt(1_000_000))
	}
	fee = fee.Add(kMakerFeePerShare.Mul(qty))
	return fee
}

func estimateTakerFee(side int8, qty, price decimal.Decimal) decimal.Decimal {
	fee := kCATFeePerTrade.Add(kBrokerFeePerTrade)
	if side == -1 {
		fee = fee.Add(kTAFFeePerShare.Mul(qty))
		fee = fee.Add(price.Mul(qty).Mul(kSECFeePerMillion).DivInt(1_000_000))
	}
	fee = fee.Add(kTakerFeePerShare.Mul(qty))
	return fee
}

func Tick(price decimal.Decimal) decimal.Decimal {
	if price.Cmp(decimal.One) < 0 {
		return decimal.Pip
	}
	return decimal.Cent
}

func quantizeBuyPrice(price decimal.Decimal) decimal.Decimal {
	return price.QuantizeTruncate(Tick(price))
}

func quantizeSellPrice(price decimal.Decimal) decimal.Decimal {
	return price.QuantizeAway(Tick(price))
}

func printResults(path string, count int) {
	fmt.Printf("\n")
	fmt.Printf("════════════════════════════════════════════════════════════════════════════\n")
	fmt.Printf("  STUDY: %s\n", path)
	fmt.Printf("════════════════════════════════════════════════════════════════════════════\n")

	// data inventory
	fmt.Printf("\n── DATA INVENTORY ──────────────────────────────────────────────────────────\n")
	fmt.Printf("  messages:   %d\n", count)
	fmt.Printf("  quotes:     %d\n", gTotalQuotes)
	fmt.Printf("  trades:     %d\n", gTotalTrades)
	fmt.Printf("  other:      %d\n", gTotalOther)
	fmt.Printf("  symbols:    %d\n", len(gSymQuote))
	fmt.Printf("  time range: %s to %s\n", gFirstTime, gLastTime)

	var top []symInventory
	for sym, qc := range gSymQuote {
		top = append(top, symInventory{sym.String(), qc, gSymTrade[sym]})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].quotes > top[j].quotes })
	fmt.Printf("\n  top 30 symbols by quote count:\n")
	fmt.Printf("  %-8s %12s %12s\n", "SYMBOL", "QUOTES", "TRADES")
	for i := 0; i < 30 && i < len(top); i++ {
		fmt.Printf("  %-8s %12d %12d\n", top[i].name, top[i].quotes, top[i].trades)
	}
	if len(top) > 30 {
		fmt.Printf("  ... and %d more symbols\n", len(top)-30)
	}

	// overall signal analysis
	fmt.Printf("\n── SIGNAL ANALYSIS (overall) ────────────────────────────────────────────────\n")
	fmt.Printf("  threshold=%s  iso=%s  greed=%s\n",
		*flagThreshold, *flagISO, *flagGreed)
	fmt.Printf("  signals: %d (buy=%d sell=%d)\n",
		gTotalSignals, gBuySignals, gSellSignals)
	printHorizonTable(gHStats[:])

	// signal quality by price bucket
	fmt.Printf("\n── BY SHARE PRICE ──────────────────────────────────────────────────────────\n")
	fmt.Printf("  which price ranges have the best signal quality?\n")
	fmt.Printf("  (size=%s floor=%s → max tradeable price = %s)\n\n",
		*flagSize, *flagFloor, flagSize.Div(*flagFloor))
	printBucketSummary(gPriceBuckets, priceBucketLabel)

	// signal quality by spread bucket
	fmt.Printf("\n── BY SPREAD (in ticks) ────────────────────────────────────────────────────\n")
	fmt.Printf("  which spread ranges have the best signal quality?\n\n")
	printBucketSummary(gSpreadBuckets, spreadBucketLabel)

	// fee economics
	// entry offset analysis
	fmt.Printf("\n── ENTRY STRATEGY (fill rate within 60s) ───────────────────────────────────\n")
	fmt.Printf("  how often does the market come to us at each entry offset?\n")
	fmt.Printf("  gross = ticks captured if filled and exit at signal-time ask\n\n")
	fmt.Printf("  %-12s %9s   %-16s %-16s %-16s\n",
		"SPREAD", "SIGNALS", "bid+1 tick", "bid+2 ticks", "bid+3 ticks")
	fmt.Printf("  %-12s %9s   %-16s %-16s %-16s\n",
		"", "", "fill   gross", "fill   gross", "fill   gross")
	for i, ea := range gEntryStats {
		if ea.signals == 0 {
			continue
		}
		label := entrySpreadLabel(i)
		// typical spread for this bucket (exact for individual ticks, min for ranges)
		var typicalSpread int
		if i < len(entrySpreadBuckets)-1 {
			typicalSpread = entrySpreadBuckets[i]
		} else {
			typicalSpread = 11 // minimum in catch-all
		}
		fmt.Printf("  %-12s %9d  ", label, ea.signals)
		for k := 0; k < 3; k++ {
			offset := k + 1
			if offset >= typicalSpread {
				fmt.Printf(" %-16s", "(cross)")
			} else {
				fillRate := float64(ea.fills[k]) / float64(ea.signals) * 100
				gross := typicalSpread - offset
				if i == len(entrySpreadBuckets)-1 {
					fmt.Printf(" %5.1f%% ≥%-6dt  ", fillRate, gross)
				} else {
					fmt.Printf(" %5.1f%% %dt       ", fillRate, gross)
				}
			}
		}
		fmt.Printf("\n")
	}

	fmt.Printf("\n── FEE ECONOMICS (per 100 shares @ $50) ────────────────────────────────────\n")
	refQty := decimal.Parse("100")
	refPrice := decimal.Parse("50")
	makerEntry := estimateFee(1, refQty, refPrice, true)
	makerExit := estimateFee(-1, refQty, refPrice, true)
	makerTotal := makerEntry.Add(makerExit)
	takerEntry := estimateTakerFee(1, refQty, refPrice)
	takerExit := estimateFee(-1, refQty, refPrice, true)
	takerTotal := takerEntry.Add(takerExit)

	fmt.Printf("  maker round trip:  %s (negative = net rebate)\n", makerTotal)
	fmt.Printf("  taker round trip:  %s\n", takerTotal)
	fmt.Printf("  taker breakeven:   %.2f bps\n",
		float64(int64(takerTotal))/float64(int64(refPrice.Mul(refQty)))*10000.0)

	// round-trip simulation
	fmt.Printf("\n── ROUND-TRIP SIMULATION ────────────────────────────────────────────────────\n")
	fmt.Printf("  size=%s  floor=%s  patience=%s  greed=%s\n",
		*flagSize, *flagFloor, *flagPatience, *flagGreed)
	fmt.Printf("  round trips: %d\n", gSimRoundTrips)
	fmt.Printf("  wins:        %d", gSimWins)
	if gSimRoundTrips > 0 {
		fmt.Printf(" (%.1f%%)", float64(gSimWins)/float64(gSimRoundTrips)*100)
	}
	fmt.Printf("\n")
	fmt.Printf("  timeouts:    %d\n", gSimTimeouts)
	net := gSimPnL.Sub(gSimFees)
	fmt.Printf("  realized:    %s\n", gSimPnL)
	fmt.Printf("  fees:        %s\n", gSimFees)
	fmt.Printf("  net P&L:     %s\n", net)
	if gSimRoundTrips > 0 {
		fmt.Printf("  per trade:   %s\n", net.Div(decimal.FromInt64(gSimRoundTrips)))
	}

	// aggregate quote and trade condition stats across all symbols
	var totalInvalidPrice, totalInvalidState uint64
	var totalInvalidTradeCondition, totalInvalidTradeMidpoint uint64
	allQuoteConditions := make(map[string]uint64)
	allTradeConditions := make(map[string]uint64)
	for _, st := range gSymbols {
		totalInvalidPrice += st.invalidQuotePrice
		totalInvalidState += st.invalidQuoteState
		for code, cnt := range st.quoteConditionCode {
			allQuoteConditions[code] += cnt
		}
		totalInvalidTradeCondition += st.invalidTradeCondition
		totalInvalidTradeMidpoint += st.invalidTradeMidpoint
		for code, cnt := range st.tradeConditionCode {
			allTradeConditions[code] += cnt
		}
	}

	fmt.Printf("\n── QUOTE CONDITION CODES ────────────────────────────────────────────────────\n")
	fmt.Printf("  invalid price (zero bid/ask): %d\n", totalInvalidPrice)
	fmt.Printf("  invalid state (bid >= ask):   %d\n", totalInvalidState)
	fmt.Printf("\n  %-30s %12s\n", "CONDITION CODE", "QUOTES")

	type condEntry struct {
		code  string
		count uint64
	}
	var condList []condEntry
	for code, cnt := range allQuoteConditions {
		condList = append(condList, condEntry{code, cnt})
	}
	sort.Slice(condList, func(i, j int) bool { return condList[i].count > condList[j].count })
	for _, e := range condList {
		fmt.Printf("  %-30s %12d\n", e.code, e.count)
	}

	fmt.Printf("\n── TRADE CONDITION CODES ────────────────────────────────────────────────────\n")
	fmt.Printf("  non-ISO trades (skipped):     %d\n", totalInvalidTradeCondition)
	fmt.Printf("  ISO trades w/ zero midpoint:  %d\n", totalInvalidTradeMidpoint)
	fmt.Printf("\n  %-30s %12s\n", "CONDITION CODE", "TRADES")

	var tradeCondList []condEntry
	for code, cnt := range allTradeConditions {
		tradeCondList = append(tradeCondList, condEntry{code, cnt})
	}
	sort.Slice(tradeCondList, func(i, j int) bool { return tradeCondList[i].count > tradeCondList[j].count })
	for _, e := range tradeCondList {
		fmt.Printf("  %-30s %12d\n", e.code, e.count)
	}

	openCount := 0
	openPnL := decimal.Zero
	for _, st := range gSymbols {
		if !st.position.IsZero() {
			openCount++
			mid := st.quote.Midpoint()
			if st.position.IsPositive() {
				avg := st.costBasis.Div(st.position)
				openPnL = openPnL.Add(mid.Sub(avg).Mul(st.position))
			} else {
				avg := st.costBasis.Div(st.position)
				openPnL = openPnL.Add(avg.Sub(mid).Mul(st.position.Neg()))
			}
		}
	}
	if openCount > 0 {
		fmt.Printf("  open positions: %d (unrealized: %s)\n", openCount, openPnL)
	}

	fmt.Printf("\n")
}

func printHorizonTable(hs []horizonStats) {
	fmt.Printf("  %-10s %10s %8s %10s %10s\n",
		"HORIZON", "SIGNALS", "HIT", "AVG MOVE", "|MOVE|")
	for h, hz := range horizons {
		s := hs[h]
		if s.count == 0 {
			continue
		}
		hitRate := float64(s.correct) / float64(s.count) * 100
		avgMove := float64(s.moveSum) / float64(s.count) / 1000.0
		avgAbsMove := float64(s.absMoveSum) / float64(s.count) / 1000.0
		fmt.Printf("  %-10s %10d %7.1f%% %8.2f bp %8.2f bp\n",
			hz, s.count, hitRate, avgMove, avgAbsMove)
	}
}

func printBucketSummary(buckets []bucketStats, labelFn func(int) string) {
	// header: bucket | signals | 1s hit | 1s move | 5s hit | 5s move | 30s hit | 30s move
	fmt.Printf("  %-14s %10s", "BUCKET", "SIGNALS")
	for _, h := range []int{2, 3, 4, 5} { // 1s, 5s, 30s, 60s
		fmt.Printf(" %8s %7s", horizons[h].String()+" hit", "move")
	}
	fmt.Printf("\n")

	for i, b := range buckets {
		if b.signals == 0 {
			continue
		}
		fmt.Printf("  %-14s %10d", labelFn(i), b.signals)
		for _, h := range []int{2, 3, 4, 5} {
			s := b.hstats[h]
			if s.count == 0 {
				fmt.Printf(" %8s %7s", "-", "-")
			} else {
				hitRate := float64(s.correct) / float64(s.count) * 100
				avgMove := float64(s.moveSum) / float64(s.count) / 1000.0
				fmt.Printf(" %7.1f%% %5.1f bp", hitRate, avgMove)
			}
		}
		fmt.Printf("\n")
	}
}

func priceBucketLabel(i int) string {
	if i == 0 {
		return fmt.Sprintf("< $%s", priceBuckets[0])
	}
	if i == len(priceBuckets)-1 {
		return fmt.Sprintf(">= $%s", priceBuckets[i-1])
	}
	return fmt.Sprintf("$%s - $%s", priceBuckets[i-1], priceBuckets[i])
}

func spreadBucketLabel(i int) string {
	if i == 0 {
		return fmt.Sprintf("%d tick", spreadBuckets[0])
	}
	if i == len(spreadBuckets)-1 {
		return fmt.Sprintf(">= %d ticks", spreadBuckets[i-1])
	}
	return fmt.Sprintf("%d-%d ticks", spreadBuckets[i-1]+1, spreadBuckets[i])
}

func recordEntryFills(sig *pendingSignal) {
	ea := &gEntryStats[sig.entryBkt]
	ea.signals++
	tick := Tick(sig.bidPrice)
	for k := 0; k < 3; k++ {
		offset := k + 1
		if offset >= sig.spreadTick {
			break // would cross the spread
		}
		if sig.direction == 1 {
			entryPrice := sig.bidPrice.Add(tick.MulInt(offset))
			if !sig.minAsk.IsZero() && sig.minAsk.Cmp(entryPrice) <= 0 {
				ea.fills[k]++
			}
		} else {
			entryPrice := sig.askPrice.Sub(tick.MulInt(offset))
			if !sig.maxBid.IsZero() && sig.maxBid.Cmp(entryPrice) >= 0 {
				ea.fills[k]++
			}
		}
	}
}

func entrySpreadLabel(i int) string {
	if i == len(entrySpreadBuckets)-1 {
		return fmt.Sprintf("≥ %d ticks", entrySpreadBuckets[i-1]+1)
	}
	return fmt.Sprintf("%d ticks", entrySpreadBuckets[i])
}

func fmtDollar(d float64) string {
	if d >= 0 {
		return fmt.Sprintf("$%.4f", d)
	}
	return fmt.Sprintf("-$%.4f", -d)
}
