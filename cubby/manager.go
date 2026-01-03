package cubby

import (
	"dropbear/clocky"
	"dropbear/indicators"
	"dropbear/loggy"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

// heapEntry is either a candle or a scheduled event.
type heapEntry struct {
	time     clocky.Time
	candle   *candleEntry // non-nil for candle data
	callback func()       // non-nil for scheduled events
}

type candleEntry struct {
	data   *recordedCandleData
	candle *indicators.Candle
	equity *Equity
}

type manager struct {
	heap        *binaryheap.Heap[*heapEntry]
	sample      func(clocky.Time)
	lock        sync.Mutex
	report      *report
	ready       bool
	finished    bool
	start       clocky.Time
	end         clocky.Time
	actualStart clocky.Time // latest first-candle across all equities
}

func newManager(report *report) *manager {
	return &manager{
		heap:   binaryheap.NewWith(compareHeapEntries),
		sample: func(clocky.Time) {},
		report: report,
		start:  StartTime(),
		end:    EndTime(),
	}
}

func (m *manager) Close() {
	for !m.heap.Empty() {
		entry, _ := m.heap.Pop()
		if entry.candle != nil {
			entry.candle.data.Close()
		}
	}
	m.finished = false
}

// checkMarginCalls checks all brokers for margin calls and triggers auto-liquidation.
func (m *manager) checkMarginCalls() {
	Brokers.Each(func(b *Broker) {
		b.CheckMarginCall()
	})
}

// checkLiquidation returns true if account equity has been wiped out.
func (m *manager) checkLiquidation() bool {
	equity := GetEquityUSD()
	if !equity.IsPositive() {
		log.Printf("[cubby] account equity is %s you got rekt", equity.Format(2))
		log.Printf("     .... NO! ...                  ... MNO! ...")
		log.Printf("   ..... MNO!! ...................... MNNOO! ...")
		log.Printf(" ..... MMNO! ......................... MNNOO!! .")
		log.Printf("..... MNOONNOO!   MMMMMMMMMMPPPOII!   MNNO!!!! .")
		log.Printf(" ... !O! NNO! MMMMMMMMMMMMMPPPOOOII!! NO! ....")
		log.Printf("    ...... ! MMMMMMMMMMMMMPPPPOOOOIII! ! ...")
		log.Printf("   ........ MMMMMMMMMMMMPPPPPOOOOOOII!! .....")
		log.Printf("   ........ MMMMMOOOOOOPPPPPPPPOOOOMII! ...")
		log.Printf("    ....... MMMMM..    OPPMMP    .,OMI! ....")
		log.Printf("     ...... MMMM::   o.,OPMP,.o   ::I!! ...")
		log.Printf("         .... NNM:::.,,OOPM!P,.::::!! ....")
		log.Printf("          .. MMNNNNNOOOOPMO!!IIPPO!!O! .....")
		log.Printf("         ... MMMMMNNNNOO:!!:!!IPPPPOO! ....")
		log.Printf("           .. MMMMMNNOOMMNNIIIPPPOO!! ......")
		log.Printf("          ...... MMMONNMMNNNIIIOO!..........")
		log.Printf("       ....... MN MOMMMNNNIIIIIO! OO ..........")
		log.Printf("    ......... MNO! IiiiiiiiiiiiI OOOO ...........")
		log.Printf("  ...... NNN.MNO! . O!!!!!!!!!O . OONO NO! ........")
		log.Printf("   .... MNNNNNO! ...OOOOOOOOOOO .  MMNNON!........")
		log.Printf("   ...... MNNNNO! .. PPPPPPPPP .. MMNON!........")
		log.Printf("      ...... OO! ................. ON! .......")
		log.Printf("         ................................")
		return true
	}
	return false
}

func (m *manager) Register(equity *Equity) {
	data := openRecordedCandleData(equity.Symbol, m.start, m.end)
	if data == nil {
		loggy.Fatalf("no data found for %s", equity.Symbol)
	}
	ce := &candleEntry{equity: equity, data: data, candle: &indicators.Candle{}}
	// Binary search to first candle >= start time
	data.SeekTo(m.start)
	err := data.Read(ce.candle)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			data.Close()
			return
		}
		loggy.Fatalf("failed to read initial candle: %v", err)
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.finished {
		panic(fmt.Sprintf("cannot register %v equity after market data manager has started running", equity))
	}
	// Track latest first-candle across all equities
	if ce.candle.Start > m.actualStart {
		m.actualStart = ce.candle.Start
	}
	m.heap.Push(&heapEntry{time: ce.candle.Start, candle: ce})
}

func (m *manager) Run() {
	m.lock.Lock()
	m.finished = true
	m.lock.Unlock()

	// Generate scheduled events for the date range
	m.generateScheduledEvents()

	oldOnReady := Brokers.OnReady
	Brokers.OnReady = func() {
		if oldOnReady != nil {
			oldOnReady()
		}
		m.report.Init()
		m.sample = m.report.Sample
		m.ready = true
	}

	// Fast-forward all equities to actualStart (when all have data)
	if m.actualStart > m.start {
		log.Printf("[cubby] fast-forwarding to %s (earliest common data)", m.actualStart)
		// Set time before fast-forward so OnReady/report.Init() sees correct time
		clocky.SetNow(m.actualStart)
		// Drain heap, seek all candle readers to actualStart, rebuild
		var entries []*heapEntry
		for !m.heap.Empty() {
			entry, _ := m.heap.Pop()
			if entry.candle != nil {
				ce := entry.candle
				ce.data.SeekTo(m.actualStart)
				err := ce.data.Read(ce.candle)
				if err != nil {
					if err == io.EOF || err == io.ErrUnexpectedEOF {
						ce.data.Close()
						continue
					}
					panic("failed to read candle: " + err.Error())
				}
				// Mark equity ready since we're past its first candle
				ce.equity.LastPrice.Store(ce.candle.Close)
				if !ce.equity.isReady {
					ce.equity.isReady = true
					ce.equity.Broker.Equities.markReady(ce.equity)
				}
				entries = append(entries, &heapEntry{time: ce.candle.Start, candle: ce})
			} else if entry.time >= m.actualStart {
				// Keep scheduled events at or after actualStart
				entries = append(entries, entry)
			}
		}
		for _, e := range entries {
			m.heap.Push(e)
		}
	}
	for !m.heap.Empty() {
		entry, _ := m.heap.Pop()
		now := entry.time

		// Stop if we've reached end time
		if now.After(m.end) {
			if entry.candle != nil {
				entry.candle.data.Close()
			}
			continue
		}

		clocky.SetNow(now)

		// Handle scheduled event
		if entry.callback != nil {
			entry.callback()
			continue
		}

		// Handle candle
		ce := entry.candle
		m.sample(now)
		ce.equity.handleCandle(ce.candle)

		if m.ready {
			ce.equity.OnCandle(ce.candle)

			// Check for margin calls (equity < maintenance margin)
			m.checkMarginCalls()

			// Check for liquidation (equity <= 0)
			if m.checkLiquidation() {
				ce.data.Close()
				// Drain remaining entries
				for !m.heap.Empty() {
					e, _ := m.heap.Pop()
					if e.candle != nil {
						e.candle.data.Close()
					}
				}
				return
			}
		}

		err := ce.data.Read(ce.candle)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				ce.data.Close()
				// Continue with remaining equities (union, not intersection)
				continue
			}
			panic("failed to read candle: " + err.Error())
		}
		m.heap.Push(&heapEntry{time: ce.candle.Start, candle: ce})
	}
	if !m.ready {
		panic("no market data available; cannot run backtest")
	}
}

// generateScheduledEvents creates all scheduled events for the date range.
func (m *manager) generateScheduledEvents() {
	// Iterate through each day in the range
	startDay := time.UnixMicro(int64(m.start)).In(Pacific)
	endDay := time.UnixMicro(int64(m.end)).In(Pacific)

	for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
		// Skip weekends
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}

		year, month, d := day.Date()

		// Market open: 6:30 AM PT
		openTime := time.Date(year, month, d, MarketOpenHour, MarketOpenMinute, 0, 0, Pacific)
		openTs := clocky.Time(openTime.UnixMicro())
		if openTs >= m.start && openTs <= m.end {
			for _, fn := range gSchedule.beforeOpen {
				cb := fn // capture
				m.heap.Push(&heapEntry{time: openTs, callback: cb})
			}
			for _, fn := range gSchedule.afterOpen {
				cb := fn
				m.heap.Push(&heapEntry{time: openTs + 1, callback: cb}) // +1µs to order after beforeOpen
			}
		}

		// Close early: 12:50 PT (ends day trading time)
		closeEarlyTime := time.Date(year, month, d, MarketCloseHour, -10, 0, 0, Pacific)
		closeEarlyTs := clocky.Time(closeEarlyTime.UnixMicro())
		if closeEarlyTs >= m.start && closeEarlyTs <= m.end {
			for _, fn := range gSchedule.beforeCloseEarly {
				cb := fn
				m.heap.Push(&heapEntry{time: closeEarlyTs, callback: cb})
			}
		}

		// Market close: 1:00 PM PT
		closeTime := time.Date(year, month, d, MarketCloseHour, MarketCloseMinute, 0, 0, Pacific)
		closeTs := clocky.Time(closeTime.UnixMicro())
		if closeTs >= m.start && closeTs <= m.end {
			for _, fn := range gSchedule.beforeClose {
				cb := fn
				m.heap.Push(&heapEntry{time: closeTs, callback: cb})
			}
			for _, fn := range gSchedule.afterClose {
				cb := fn
				m.heap.Push(&heapEntry{time: closeTs + 1, callback: cb})
			}
		}
	}
}

func compareHeapEntries(a, b *heapEntry) int {
	if a.time < b.time {
		return -1
	}
	if a.time > b.time {
		return +1
	}
	return 0
}
