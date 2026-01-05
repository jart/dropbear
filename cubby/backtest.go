package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"log"
	"os"
	"path"

	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

type backtest struct {
	interest  *alpaca.InterestCalculator
	heap      *binaryheap.Heap[*backtestEntry]
	minTime   clocky.Time
	maxTime   clocky.Time
	startCash decimal.Decimal
	opened    bool
	closed    bool
	date      int
}

type backtestEntry struct {
	bar    *alpaca.Bar
	bars   *Bars
	equity *Equity
}

const (
	openTime  = 6_30_00
	closeTime = 13_00_00
)

func newBacktest() *backtest {
	m := &backtest{
		interest:  alpaca.NewInterestCalculator(decimal.Parse("0.01")),
		heap:      binaryheap.NewWith(compareBacktestEntries),
		minTime:   *flagStart,
		maxTime:   *flagEnd,
		startCash: Cash,
	}
	if len(Equities) == 0 {
		panic("no equities registered for backtest")
	}
	entries := make([]*backtestEntry, 0, len(Equities))
	equityData := getEquityDataDir()
	for _, equity := range Equities {
		path := path.Join(equityData, "minutes", equity.Symbol)
		bars, err := OpenBars(path)
		if err != nil {
			panic(err)
		}
		first := bars.Get(0)
		last := bars.Get(bars.Count() - 1)
		m.minTime = max(m.minTime, first.Timestamp)
		m.maxTime = min(m.maxTime, last.Timestamp.Add(clocky.Minute))
		entry := &backtestEntry{equity: equity, bars: bars}
		entries = append(entries, entry)
	}
	if !m.minTime.Before(m.maxTime) {
		panic("no overlapping data range across all registered equities")
	}
	for _, entry := range entries {
		entry.bars.Seek(m.minTime)
		entry.bar = entry.bars.Read()
		if entry.bar == nil || entry.bar.Timestamp.After(m.maxTime) {
			panic("equity has no data in overlapping range: " + entry.equity.Symbol)
		}
		m.heap.Push(entry)
	}
	gMaxMarginAvailable = Cash
	gPowerLevel = decimal.FromInt(1) // start with overnight margin (1x)
	return m
}

func (m *backtest) Close() {
	for !m.heap.Empty() {
		entry, _ := m.heap.Pop()
		err := entry.bars.Close()
		if err != nil {
			panic(err)
		}
	}
}

func (m *backtest) Run() {
	var entries []*backtestEntry
	isDone := false
	iterations := 0
	for !isDone {
		iterations++
		entry, _ := m.heap.Pop()
		time := entry.bar.Timestamp
		entries = entries[:0]
		entries = append(entries, entry)
		for {
			entry, _ := m.heap.Peek()
			if entry == nil {
				break
			}
			if entry.bar.Timestamp != time {
				break
			}
			entry, _ = m.heap.Pop()
			entries = append(entries, entry)
		}
		for _, entry := range entries {
			entry.equity.Price = entry.bar.Close
			entry.equity.AskPrice = entry.bar.Close.Add(decimal.Cent)
			entry.equity.BidPrice = entry.bar.Close.Sub(decimal.Cent)
			entry.equity.nextBar = entry.bars.Peek()
		}
		m.setTime(time.Add(clocky.Minute))
		for _, entry := range entries {
			entry.equity.OnBar(entry.bar)
		}
		for _, entry := range entries {
			bar := entry.bars.Read()
			isDone = isDone || bar == nil || bar.Timestamp >= m.maxTime
			if !isDone {
				entry.bar = bar
			}
			m.heap.Push(entry)
		}
	}
	log.Printf("Backtest completed: %d iterations", iterations)

	// print summary
	endValue := GetPortfolioValue()
	totalReturn := endValue.Sub(m.startCash).Div(m.startCash)
	days := float64(m.maxTime.Sub(m.minTime)) / float64(clocky.Day)
	years := days / 365.25
	if years > 0 {
		annualReturn := (endValue.Float64()/m.startCash.Float64() - 1) * 100 / years
		log.Printf("Summary:")
		log.Printf("  Start:    $%s", m.startCash.FormatThousand(2))
		log.Printf("  End:      $%s", endValue.FormatThousand(2))
		log.Printf("  Fees:     $%s", gFeeCalculator.TotalFees.FormatThousand(2))
		log.Printf("  Slippage: $%s", gTotalSlippage.FormatThousand(2))
		log.Printf("  Interest: $%s", m.interest.TotalCharged.FormatThousand(2))
		log.Printf("  Return:   %s%% (%.1f%% annualized)", totalReturn.MulInt(100).Format(2), annualReturn)
		log.Printf("  Period:   %.1f days (%.2f years)", days, years)
	}
}

func (m *backtest) setTime(now clocky.Time) {
	clocky.SetNow(now)
	date := now.DateInt()
	if date != m.date {
		m.date = date
		m.opened = false
		m.closed = false
	}
	time := now.ClockInt()
	if !m.opened && time >= openTime && time < closeTime {
		m.opened = true
		m.onMarketOpen()
	}
	if !m.closed && time >= closeTime {
		m.closed = true
		m.onMarketClose(now)
	}
}

func (m *backtest) onMarketOpen() {
	// Update max margin at market open, when positions should be closed
	gMaxMarginAvailable = GetPortfolioValue().Sub(GetMarginUsed())
	if GetPortfolioValue().Cmp(decimal.FromInt(25_000)) > 0 {
		gPowerLevel = decimal.FromInt(2) // PDT: 4x buying power / 2 (50% margin) = 2x margin multiplier
	} else {
		gPowerLevel = decimal.FromInt(1) // Reg-T: 2x buying power / 2 (50% margin) = 1x margin multiplier
	}
}

func (m *backtest) onMarketClose(now clocky.Time) {
	gPowerLevel = decimal.FromInt(1) // overnight: 2x buying power / 2 (50% margin) = 1x
	Cash = Cash.Sub(m.interest.GetDailyInterest(now, Cash, *flagRFR))
}

func compareBacktestEntries(a, b *backtestEntry) int {
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

func getEquityDataDir() string {
	dirs := []string{
		os.ExpandEnv("$HOME/equitydata"),
		"/fast/equitydata",
		"/usr/local/share/equitydata",
	}
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	panic("no equity data directory found")
}
