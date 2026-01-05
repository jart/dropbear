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
	interest    *alpaca.InterestCalculator
	heap        *binaryheap.Heap[*backtestEntry]
	minTime     clocky.Time
	maxTime     clocky.Time
	startCash   decimal.Decimal
	opened      bool
	closed      bool
	date        int
	peakValue   decimal.Decimal // highest portfolio value seen
	maxDrawdown decimal.Decimal // largest drawdown from peak (as decimal, e.g. 0.10 = 10%)
}

type backtestEntry struct {
	bar    *alpaca.Bar
	bars   *alpaca.Bars
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
		path := path.Join(equityData, "minutes", equity.Symbol)
		bars, err := alpaca.OpenBars(path)
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
			entry.equity.AskPrice = entry.bar.Close.Add(decimal.Cent)
			entry.equity.BidPrice = entry.bar.Close.Sub(decimal.Cent)
			entry.equity.nextBar = entry.bars.Peek()
		}
		m.setTime(time.Add(clocky.Minute))
		for _, entry := range entries {
			entry.equity.OnBar(entry.bar)
		}

		// Check for liquidation after processing bars
		if m.checkLiquidation() {
			return
		}

		// Track max drawdown
		currentValue := GetPortfolioValue()
		if currentValue.Cmp(m.peakValue) > 0 {
			m.peakValue = currentValue
		} else if m.peakValue.IsPositive() {
			drawdown := m.peakValue.Sub(currentValue).Div(m.peakValue)
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
	log.Printf("Backtest completed: %d iterations", iterations)
	m.printSummary()
}

func (m *backtest) checkLiquidation() bool {
	portfolioValue := GetPortfolioValue()
	if portfolioValue.Cmp(*flagRekt) <= 0 {
		Liquidated = true
		log.Printf("portfolio value $%s fell below $%s", portfolioValue.FormatThousand(2), flagRekt.FormatThousand(2))
		rekt()
		return true
	}
	return false
}

func (m *backtest) printSummary() {
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
		log.Printf("  Max DD:   %s%%", m.maxDrawdown.MulInt(100).Format(2))
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
	gMaxMarginAvailable = GetPortfolioValue().Sub(GetMarginUsed())
	if GetPortfolioValue().Cmp(decimal.FromInt(25_000)) > 0 {
		gPowerLevel = decimal.Two
	} else {
		gPowerLevel = decimal.One
	}
}

func (m *backtest) onMarketClose(now clocky.Time) {
	gPowerLevel = decimal.One
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
