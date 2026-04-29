package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/ds"
	"dropbear/symbol"
	"log"

	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

const (
	kWarmupBars = 10000 // number of historical bars to fetch for warmup
)

type liveTrader struct {
	symbols []symbol.Symbol
	opened  bool
	closed  bool
}

func newLiveTrader() *liveTrader {
	symbols := make([]symbol.Symbol, 0, len(Equities))
	for sym := range Equities {
		symbols = append(symbols, sym)
	}
	if len(symbols) == 0 {
		panic("no equities registered for live trading")
	}
	return &liveTrader{symbols: symbols}
}

func (lt *liveTrader) Close() error {
	Client.CancelAllOrders()
	return nil
}

func (lt *liveTrader) Run() {
	lt.sync()
	lt.warmup()
	lt.loop()
}

// sync should only be called once at the start of live trading.
func (lt *liveTrader) sync() {

	log.Printf("fetching alpaca assets database...")
	Client.SyncAssets()

	// check account state
	log.Printf("fetching alpaca account state...")
	account, err := Client.GetAccount()
	if err != nil {
		panic(err)
	}
	Cash = account.Cash
	gIsPatternDayTrader = account.PatternDayTrader
	gDayTradingBuyingPower = account.BODDTBP
	gLastEquity = account.LastEquity
	gLastMaintenanceMargin = account.LastMaintenanceMargin

	// check if open orders exist
	log.Printf("checking for open orders on alpaca account...")
	alpacaOrders, err := Client.GetOrders(nil)
	if err != nil {
		panic(err)
	}
	if len(alpacaOrders) > 0 {
		panic("won't start while alpaca account has open orders")
	}

	// fetch portfolio positions
	log.Printf("fetching alpaca portfolio positions...")
	positions, err := Client.GetPositions()
	if err != nil {
		panic(err)
	}
	for _, position := range positions {
		symbol, err := symbol.Parse(position.Symbol)
		if err != nil {
			panic("found position with symbol we can't decode: " + position.Symbol)
		}
		equity, err := AddEquity(symbol)
		if err != nil {
			panic(err)
		}
		equity.Price = position.CurrentPrice
		equity.Quantity = position.QtyAvailable
		equity.EntryPrice = position.AvgEntryPrice
	}
}

// reconcile ensures that local state matches Alpaca's state.
func (lt *liveTrader) reconcile() {
	// todo
}

// warmup fetches historical bars and replays them to initialize indicators.
func (lt *liveTrader) warmup() {
	IsWarmingUp = true
	defer func() { IsWarmingUp = false }()

	// Fetch last kWarmupBars minutes of data for all symbols
	// End at the most recent closed minute (not the current incomplete minute)
	now := clocky.Now()
	end := now.Quantize(clocky.Minute)
	start := end.Add(-clocky.Duration(kWarmupBars) * clocky.Minute)

	log.Printf("warming up: fetching %d bars from %s to %s", kWarmupBars, start.RFC3339(), end.RFC3339())

	allBars, err := lt.fetchAllBars(start, end)
	if err != nil {
		log.Printf("error fetching warmup bars: %v", err)
		return
	}

	if len(allBars) == 0 {
		log.Printf("warning: no warmup bars fetched")
		return
	}

	// build a heap to replay bars in chronological order (like backtest)
	heap := binaryheap.NewWith(compareLiveEntries)
	for sym, bars := range allBars {
		equity := Equities[sym]
		if equity != nil {
			for i := range bars {
				heap.Push(&liveEntry{bar: &bars[i], equity: equity})
			}
		}
	}

	// replay bars in order
	var entries []*liveEntry
	for !heap.Empty() {
		entry, _ := heap.Pop()
		time := entry.bar.Timestamp
		entries = entries[:0]
		entries = append(entries, entry)

		// collect all bars with the same timestamp
		for {
			next, _ := heap.Peek()
			if next == nil || next.bar.Timestamp != time {
				break
			}
			entry, _ = heap.Pop()
			entries = append(entries, entry)
		}

		// update prices
		for _, e := range entries {
			e.equity.Price = e.bar.Close
		}

		// dispatch OnBar callbacks
		for _, e := range entries {
			e.equity.OnBar(e.bar)
		}
	}

	log.Printf("warmup complete: replayed bars for %d symbols", len(allBars))
}

func (lt *liveTrader) loop() {
	log.Printf("starting live trading loop")

	// track the last minute we processed
	lastProcessed := clocky.Now().Quantize(clocky.Minute)

	for {
		// wait until the current minute closes
		now := clocky.Now()
		currentMinute := now.Quantize(clocky.Minute)
		nextMinute := currentMinute.Add(clocky.Minute)

		// sleep until a few seconds after the minute closes to ensure data is available
		sleepUntil := nextMinute.Add(2 * clocky.Second)
		sleepDuration := sleepUntil.Sub(now)
		if sleepDuration > 0 {
			clocky.Sleep(sleepDuration)
		}

		// Fetch bars from lastProcessed to now
		now = clocky.Now()
		end := now.Quantize(clocky.Minute)
		start := lastProcessed

		if !start.Before(end) {
			continue // no new bars to process
		}

		bars, err := lt.fetchAllBars(start, end)
		if err != nil {
			log.Printf("error fetching bars: %v", err)
			clocky.Sleep(5 * clocky.Second)
			continue
		}
		if len(bars) == 0 {
			lastProcessed = end
			continue
		}

		// make sure we're modeling alpaca correctly
		lt.reconcile()

		// build heap for ordered replay
		heap := binaryheap.NewWith(compareLiveEntries)
		for sym, symbolBars := range bars {
			equity := Equities[sym]
			if equity == nil {
				continue
			}
			for i := range symbolBars {
				heap.Push(&liveEntry{bar: &symbolBars[i], equity: equity})
			}
		}

		// process bars in chronological order
		var entries []*liveEntry
		for !heap.Empty() {
			entry, _ := heap.Pop()
			barTime := entry.bar.Timestamp
			entries = entries[:0]
			entries = append(entries, entry)

			// collect all bars with the same timestamp
			for {
				next, _ := heap.Peek()
				if next == nil || next.bar.Timestamp != barTime {
					break
				}
				entry, _ = heap.Pop()
				entries = append(entries, entry)
			}

			// update prices
			for _, e := range entries {
				e.equity.Price = e.bar.Close
			}

			// dispatch callbacks
			for _, e := range entries {
				e.equity.OnBar(e.bar)
			}

			// update lastProcessed
			if barTime.Add(clocky.Minute).After(lastProcessed) {
				lastProcessed = barTime.Add(clocky.Minute)
			}
		}
	}
}

// fetchAllBars fetches all bars for all symbols, handling pagination.
func (lt *liveTrader) fetchAllBars(start, end clocky.Time) (map[symbol.Symbol][]ds.Bar, error) {
	allBars := make(map[symbol.Symbol][]ds.Bar)
	pageToken := ""
	for {
		bars, nextToken, err := Client.GetBarsForSymbols(
			lt.symbols,
			clocky.Minute,
			start,
			end,
			alpaca.FeedSIP,
			alpaca.BarAdjustmentRaw,
			10000, // max limit
			false, // ascending order
			pageToken,
		)
		if err != nil {
			return nil, err
		}
		for sym, symbolBars := range bars {
			allBars[sym] = append(allBars[sym], symbolBars...)
		}
		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}
	return allBars, nil
}

type liveEntry struct {
	bar    *ds.Bar
	equity *Equity
}

func compareLiveEntries(a, b *liveEntry) int {
	if a.bar.Timestamp < b.bar.Timestamp {
		return -1
	}
	if a.bar.Timestamp > b.bar.Timestamp {
		return +1
	}
	if a.equity.Symbol < b.equity.Symbol {
		return -1
	}
	if a.equity.Symbol > b.equity.Symbol {
		return +1
	}
	return 0
}
