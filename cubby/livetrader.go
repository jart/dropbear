package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"log"

	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

const (
	kWarmupBars = 1000 // number of historical bars to fetch for warmup
)

type liveTrader struct {
	interest *alpaca.InterestCalculator
	symbols  []string
	date     int
	opened   bool
	closed   bool
}

func newLiveTrader() *liveTrader {
	symbols := make([]string, 0, len(Equities))
	for symbol := range Equities {
		symbols = append(symbols, symbol)
	}
	if len(symbols) == 0 {
		panic("no equities registered for live trading")
	}
	gMaxMarginAvailable = Cash
	gPowerLevel = decimal.One
	return &liveTrader{
		interest: alpaca.NewInterestCalculator(decimal.Parse("0.01")),
		symbols:  symbols,
	}
}

func (lt *liveTrader) Close() error {
	return Client.CancelAllOrders()
}

func (lt *liveTrader) Run() {
	lt.warmup()
	lt.loop()
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

	log.Printf("Warming up: fetching %d bars from %s to %s", kWarmupBars, start.RFC3339(), end.RFC3339())

	allBars, err := lt.fetchAllBars(start, end)
	if err != nil {
		log.Printf("error fetching warmup bars: %v", err)
		return
	}

	if len(allBars) == 0 {
		log.Printf("warning: no warmup bars fetched")
		return
	}

	// Build a heap to replay bars in chronological order (like backtest)
	heap := binaryheap.NewWith(compareLiveEntries)
	for symbol, bars := range allBars {
		equity := Equities[symbol]
		if equity == nil {
			continue
		}
		for i := range bars {
			heap.Push(&liveEntry{bar: &bars[i], equity: equity})
		}
	}

	// Replay bars in order
	var entries []*liveEntry
	for !heap.Empty() {
		entry, _ := heap.Pop()
		time := entry.bar.Timestamp
		entries = entries[:0]
		entries = append(entries, entry)

		// Collect all bars with the same timestamp
		for {
			next, _ := heap.Peek()
			if next == nil || next.bar.Timestamp != time {
				break
			}
			entry, _ = heap.Pop()
			entries = append(entries, entry)
		}

		// Update prices
		for _, e := range entries {
			e.equity.Price = e.bar.Close
		}

		// Dispatch OnBar callbacks
		for _, e := range entries {
			e.equity.OnBar(e.bar)
		}
	}

	log.Printf("Warmup complete: replayed bars for %d symbols", len(allBars))
}

// loop is the main live trading loop that polls for new bars.
func (lt *liveTrader) loop() {
	log.Printf("Starting live trading loop")

	// Track the last minute we processed
	lastProcessed := clocky.Now().Quantize(clocky.Minute)

	for {
		// Wait until the current minute closes
		now := clocky.Now()
		currentMinute := now.Quantize(clocky.Minute)
		nextMinute := currentMinute.Add(clocky.Minute)

		// Sleep until a few seconds after the minute closes to ensure data is available
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

		// Build heap for ordered replay
		heap := binaryheap.NewWith(compareLiveEntries)
		for symbol, symbolBars := range bars {
			equity := Equities[symbol]
			if equity == nil {
				continue
			}
			for i := range symbolBars {
				heap.Push(&liveEntry{bar: &symbolBars[i], equity: equity})
			}
		}

		// Process bars in chronological order
		var entries []*liveEntry
		for !heap.Empty() {
			entry, _ := heap.Pop()
			time := entry.bar.Timestamp
			entries = entries[:0]
			entries = append(entries, entry)

			// Collect all bars with the same timestamp
			for {
				next, _ := heap.Peek()
				if next == nil || next.bar.Timestamp != time {
					break
				}
				entry, _ = heap.Pop()
				entries = append(entries, entry)
			}

			// Update prices
			for _, e := range entries {
				e.equity.Price = e.bar.Close
				e.equity.nextBar = nil
			}

			lt.setTime(time.Add(clocky.Minute))

			// Dispatch OnBar callbacks
			for _, e := range entries {
				e.equity.OnBar(e.bar)
			}

			// Update lastProcessed
			if time.Add(clocky.Minute).After(lastProcessed) {
				lastProcessed = time.Add(clocky.Minute)
			}
		}
	}
}

// fetchAllBars fetches all bars for all symbols, handling pagination.
func (lt *liveTrader) fetchAllBars(start, end clocky.Time) (map[string][]ds.Bar, error) {
	allBars := make(map[string][]ds.Bar)
	pageToken := ""
	for {
		bars, nextToken, err := Client.GetBarsForSymbols(
			lt.symbols,
			clocky.Minute,
			start,
			end,
			alpaca.DataFeedSIP,
			alpaca.BarAdjustmentRaw,
			10000, // max limit
			false, // ascending order
			pageToken,
		)
		if err != nil {
			return nil, err
		}
		for symbol, symbolBars := range bars {
			allBars[symbol] = append(allBars[symbol], symbolBars...)
		}
		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}
	return allBars, nil
}

func (lt *liveTrader) setTime(now clocky.Time) {
	date := now.DateInt()
	if date != lt.date {
		lt.date = date
		lt.opened = false
		lt.closed = false
	}
	time := now.ClockInt()
	if !lt.opened && time >= openTime && time < closeTime {
		lt.opened = true
		lt.onMarketOpen()
	}
	if !lt.closed && time >= closeTime {
		lt.closed = true
		lt.onMarketClose(now)
	}
}

func (lt *liveTrader) onMarketOpen() {
	gMaxMarginAvailable = GetPortfolioValue().Sub(GetMarginUsed())
	if GetPortfolioValue().Cmp(decimal.FromInt(25_000)) > 0 {
		gPowerLevel = decimal.Two
	} else {
		gPowerLevel = decimal.One
	}
	log.Printf("Market open: portfolio $%s, margin available $%s, power level %sx",
		GetPortfolioValue().FormatThousand(2),
		gMaxMarginAvailable.FormatThousand(2),
		gPowerLevel.Format(0))
}

func (lt *liveTrader) onMarketClose(now clocky.Time) {
	gPowerLevel = decimal.One
	interest := lt.interest.GetDailyInterest(now, Cash, *flagRFR)
	Cash = Cash.Sub(interest)
	log.Printf("Market close: portfolio $%s, interest charged $%s",
		GetPortfolioValue().FormatThousand(2),
		interest.Format(2))
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
