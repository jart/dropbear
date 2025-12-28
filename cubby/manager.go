package cubby

import (
	"dropbear/clocky"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/emirpasic/gods/v2/trees/binaryheap"
	"github.com/klauspost/compress/zstd"
)

type managerEntry struct {
	data   *recordedCandleData
	candle *indicators.Candle
	equity *Equity
}

type manager struct {
	heap     *binaryheap.Heap[*managerEntry]
	sample   func(clocky.Time)
	lock     sync.Mutex
	report   *report
	ready    bool
	finished bool
	start    clocky.Time
	end      clocky.Time
}

func newManager(report *report) *manager {
	return &manager{
		heap:   binaryheap.NewWith(compareManagerEntries),
		sample: func(clocky.Time) {},
		report: report,
		start:  StartTime(),
		end:    EndTime(),
	}
}

func (m *manager) Close() {
	for !m.heap.Empty() {
		entry, _ := m.heap.Pop()
		entry.data.Close()
	}
	m.finished = false
}

func (m *manager) Register(equity *Equity) {
	data := openRecordedCandleData(equity.Symbol, m.start, m.end)
	if data == nil {
		loggy.Fatalf("no data found for %s", equity.Symbol)
	}
	entry := &managerEntry{equity: equity, data: data, candle: &indicators.Candle{}}
	// Skip candles before start time
	for {
		err := data.Read(entry.candle)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				data.Close()
				return
			}
			loggy.Fatalf("failed to read initial candle: %v", err)
		}
		if !entry.candle.Start.Before(m.start) {
			break
		}
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.finished {
		panic(fmt.Sprintf("cannot register %v equity after market data manager has started running", equity))
	}
	m.heap.Push(entry)
}

func (m *manager) Run() {
	m.lock.Lock()
	m.finished = true
	m.lock.Unlock()
	oldOnReady := Exchanges.OnReady
	Exchanges.OnReady = func() {
		if oldOnReady != nil {
			oldOnReady()
		}
		m.report.Init()
		m.sample = m.report.Sample
		m.ready = true
	}
	for !m.heap.Empty() {
		entry, _ := m.heap.Pop()
		now := entry.candle.Start

		// Stop if we've reached end time
		if now.After(m.end) {
			entry.data.Close()
			continue
		}

		clocky.SetNow(now)

		m.sample(now)
		entry.equity.handleCandle(entry.candle)

		if m.ready {
			entry.equity.OnCandle(entry.candle)
		}

		err := entry.data.Read(entry.candle)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				entry.data.Close()
				// Continue with remaining equities (union, not intersection)
				continue
			}
			panic("failed to read candle: " + err.Error())
		}
		m.heap.Push(entry)
	}
	if !m.ready {
		panic("no market data available; cannot run backtest")
	}
}

func compareManagerEntries(a, b *managerEntry) int {
	if a.candle.Start.Before(b.candle.Start) {
		return -1
	}
	if a.candle.Start.After(b.candle.Start) {
		return +1
	}
	return 0
}

// recordedCandleData reads candles from multiple monthly files for a symbol.
type recordedCandleData struct {
	files   []*os.File
	readers []*zstd.Decoder
	current int
}

// openRecordedCandleData opens all data files for a symbol within the date range.
func openRecordedCandleData(symbol string, start, end clocky.Time) *recordedCandleData {
	base := ds.EquityMinutesDir()
	if base == "" {
		loggy.Fatalf("equitydata directory not found")
	}
	symbolDir := filepath.Join(base, symbol)

	// List all monthly files for this symbol
	entries, err := os.ReadDir(symbolDir)
	if err != nil {
		return nil
	}

	// Parse start/end into year-month for filtering
	startYM := time.UnixMicro(int64(start)).Format("2006-01")
	endYM := time.UnixMicro(int64(end)).Format("2006-01")

	// Open files within range
	var files []*os.File
	var readers []*zstd.Decoder
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name() // Format: YYYY-MM
		if len(name) < 7 {
			continue
		}
		// Filter to files within our date range
		if name < startYM || name > endYM {
			continue
		}
		path := filepath.Join(symbolDir, name)
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		reader, err := zstd.NewReader(file)
		if err != nil {
			file.Close()
			continue
		}
		files = append(files, file)
		readers = append(readers, reader)
	}

	if len(files) == 0 {
		return nil
	}

	// Sort by filename (chronological order)
	indices := make([]int, len(files))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		return files[indices[i]].Name() < files[indices[j]].Name()
	})
	sortedFiles := make([]*os.File, len(files))
	sortedReaders := make([]*zstd.Decoder, len(readers))
	for i, idx := range indices {
		sortedFiles[i] = files[idx]
		sortedReaders[i] = readers[idx]
	}

	return &recordedCandleData{
		files:   sortedFiles,
		readers: sortedReaders,
		current: 0,
	}
}

func (m *recordedCandleData) Read(candle *indicators.Candle) error {
	for m.current < len(m.readers) {
		err := candle.Deserialize(m.readers[m.current])
		if err == nil {
			return nil
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			m.current++
			continue
		}
		return err
	}
	return io.EOF
}

func (m *recordedCandleData) Close() {
	for i := range m.readers {
		m.readers[i].Close()
		m.files[i].Close()
	}
}
