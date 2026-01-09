package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"log"
	"os"
	"path"

	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

type backtest struct {
	interest       *alpaca.InterestCalculator
	heap           *binaryheap.Heap[*backtestEntry]
	minTime        clocky.Time
	maxTime        clocky.Time
	startCash      decimal.Decimal
	opened         bool
	closed         bool
	date           int
	peakValue      decimal.Decimal // highest portfolio value seen
	maxDrawdown    decimal.Decimal // largest drawdown from peak (as decimal, e.g. 0.10 = 10%)
	benchmarkStart decimal.Decimal // benchmark price at start of backtest
}

type backtestEntry struct {
	bar    *ds.Bar
	bars   *ds.Bars
	equity *Equity
}

func newBacktest() *backtest {
	m := &backtest{
		interest:  alpaca.NewInterestCalculator(decimal.Parse("0.01")),
		heap:      binaryheap.NewWith(compareBacktestEntries),
		minTime:   clocky.MaxTime,
		maxTime:   clocky.MinTime,
		startCash: Cash,
	}
	if len(Equities) == 0 {
		panic("no equities registered for backtest")
	}
	entries := make([]*backtestEntry, 0, len(Equities))
	equityData := getEquityDataDir()
	for _, equity := range Equities {
		path := path.Join(equityData, "minutes", equity.Symbol.String())
		bars, err := ds.OpenBars(path)
		if err != nil {
			panic(err)
		}
		first := bars.Get(0)
		last := bars.Get(bars.Count() - 1)
		m.minTime = min(m.minTime, first.Timestamp)
		m.maxTime = max(m.maxTime, last.Timestamp.Add(clocky.Minute))
		entry := &backtestEntry{equity: equity, bars: bars}
		entries = append(entries, entry)
	}
	m.minTime = max(m.minTime, *flagStart)
	m.maxTime = min(m.maxTime, *flagEnd)
	if !m.minTime.Before(m.maxTime) {
		panic("no overlapping data range across all registered equities")
	}
	for _, entry := range entries {
		entry.bars.Seek(m.minTime)
		entry.bar = entry.bars.Read()
		if entry.bar == nil || entry.bar.Timestamp.After(m.maxTime) {
			log.Printf("equity has no data in overlapping range: %s", entry.equity.Symbol)
			entry.bars.Close()
			continue
		}
		m.heap.Push(entry)
	}
	gMaxMarginAvailable = Cash
	gPowerLevel = decimal.FromInt(1) // start with overnight margin (1x)
	m.peakValue = Cash
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
	iterations := 0
	for !m.heap.Empty() {
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
		}
		if m.benchmarkStart.IsZero() && Benchmark != nil && Benchmark.Price.IsPositive() {
			m.benchmarkStart = Benchmark.Price
		}
		now := time.Add(clocky.Minute)
		m.setTime(now)
		for _, entry := range entries {
			for _, order := range entry.equity.Orders {
				order.simulateFill(now, entry.bar)
			}
		}
		for _, entry := range entries {
			entry.equity.OnBar(entry.bar)
		}
		portfolioValue := GetPortfolioValue()
		if portfolioValue.Cmp(*flagRekt) <= 0 {
			Liquidated = true
			log.Printf("rekt: portfolio value $%s fell below $%s", portfolioValue.FormatThousand(2), flagRekt.FormatThousand(2))
			rekt()
		}
		if portfolioValue.Cmp(m.peakValue) > 0 {
			m.peakValue = portfolioValue
		} else if m.peakValue.IsPositive() {
			drawdown := m.peakValue.Sub(portfolioValue).Div(m.peakValue)
			if drawdown.Cmp(m.maxDrawdown) > 0 {
				m.maxDrawdown = drawdown
			}
		}
		for _, entry := range entries {
			entry.bar = entry.bars.Read()
			if entry.bar == nil || entry.bar.Timestamp >= m.maxTime {
				entry.bars.Close()
			} else {
				m.heap.Push(entry)
			}
		}
	}
	log.Printf("backtest completed: %d iterations", iterations)
	m.printSummary()
}

func (m *backtest) printSummary() {
	endValue := GetPortfolioValue()
	totalReturn := endValue.Sub(m.startCash).Div(m.startCash)
	days := float64(m.maxTime.Sub(m.minTime)) / float64(clocky.Day)
	years := days / 365.25
	if years > 0 {
		annualReturn := (endValue.Float64()/m.startCash.Float64() - 1) * 100 / years
		log.Printf("summary:")
		log.Printf("  start:    $%s", m.startCash.FormatThousand(2))
		log.Printf("  end:      $%s", endValue.FormatThousand(2))
		log.Printf("  fees:     $%s", gFeeCalculator.TotalFees.FormatThousand(2))
		log.Printf("  interest: $%s", m.interest.TotalCharged.FormatThousand(2))
		log.Printf("  max dd:   %s%%", m.maxDrawdown.MulInt(100).Format(2))
		log.Printf("  return:   %s%% (%.2f%% annualized)", totalReturn.MulInt(100).Format(2), annualReturn)
		if Benchmark != nil && m.benchmarkStart.IsPositive() {
			benchReturn := Benchmark.Price.Sub(m.benchmarkStart).Div(m.benchmarkStart)
			benchAnnual := (Benchmark.Price.Float64()/m.benchmarkStart.Float64() - 1) * 100 / years
			log.Printf("  bench:    %s%% (%.2f%% annualized) [%s]", benchReturn.MulInt(100).Format(2), benchAnnual, Benchmark.Symbol)
		}
		log.Printf("  period:   %.1f days (%.2f years)", days, years)
		log.Printf("holdings:")
		log.Printf("%8s $%s margin $%s", "USD", Cash.FormatThousand(2), GetMarginUsed().FormatThousand(2))
		for _, equity := range Equities {
			if !equity.Quantity.IsZero() {
				log.Printf("%8s %8s shares @ $%s = $%s", equity.Symbol, equity.Quantity.Format(0), equity.Price.Format(2), equity.Price.Mul(equity.Quantity).FormatThousand(2))
			}
		}
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
	openTime := GetOpenTime(now)
	closeTime := GetCloseTime(now)
	if !m.opened && !now.Before(openTime) && now.Before(closeTime) {
		m.opened = true
		m.onMarketOpen()
	}
	if !m.closed && !now.Before(closeTime) {
		m.closed = true
		m.onMarketClose(now)
	}
}

func (m *backtest) onMarketOpen() {
	gMaxMarginAvailable = GetPortfolioValue().Sub(GetMarginUsed())
	if GetPortfolioValue().Cmp(decimal.FromInt(25_000)) > 0 {
		gPowerLevel = decimal.Two
	} else {
		gPowerLevel = decimal.One
	}
}

func (m *backtest) onMarketClose(now clocky.Time) {
	for _, order := range Orders {
		order.Cancel()
	}
	gPowerLevel = decimal.One
	Cash = Cash.Sub(m.interest.GetDailyInterest(now, Cash, *flagRFR))
	margin := GetMarginUsed()
	equity := GetPortfolioValue()
	if margin.Cmp(equity) > 0 {
		log.Printf("margin call: margin after close $%s exceeds equity $%s", margin.FormatThousand(2), equity.FormatThousand(2))
		os.Exit(1)
	}
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
