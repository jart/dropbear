package cubby

import (
	"dropbear/clocky"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"

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

// checkMarginCalls checks all brokers for margin calls and triggers auto-liquidation.
func (m *manager) checkMarginCalls() {
	for _, b := range Brokers.All() {
		b.CheckMarginCall()
	}
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
	oldOnReady := Brokers.OnReady
	Brokers.OnReady = func() {
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

		// fire scheduled callbacks
		checkSchedule(now)

		m.sample(now)
		entry.equity.handleCandle(entry.candle)

		if m.ready {
			entry.equity.OnCandle(entry.candle)

			// Check for margin calls (equity < maintenance margin)
			m.checkMarginCalls()

			// Check for liquidation (equity <= 0)
			if m.checkLiquidation() {
				entry.data.Close()
				// Drain remaining entries
				for !m.heap.Empty() {
					e, _ := m.heap.Pop()
					e.data.Close()
				}
				return
			}
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

// recordedCandleData reads candles one month at a time to limit memory usage.
// Each month is slurped into memory, then the file is closed.
type recordedCandleData struct {
	symbolDir string
	files     []string // remaining monthly files to read
	candles   []indicators.Candle
	current   int
}

// openRecordedCandleData prepares to read candles for a symbol within the date range.
// Files are loaded one month at a time to avoid file descriptor limits.
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
	startYM := start.YearMonth()
	endYM := end.YearMonth()

	// Collect filenames within range
	var filenames []string
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
		filenames = append(filenames, name)
	}

	if len(filenames) == 0 {
		return nil
	}

	// Sort filenames chronologically
	sort.Strings(filenames)

	m := &recordedCandleData{
		symbolDir: symbolDir,
		files:     filenames,
	}

	// Load first month
	m.loadNextMonth()
	if len(m.candles) == 0 {
		return nil
	}

	return m
}

// loadNextMonth loads the next month's candles into memory.
func (m *recordedCandleData) loadNextMonth() {
	m.candles = nil
	m.current = 0
	for len(m.files) > 0 {
		name := m.files[0]
		m.files = m.files[1:]
		path := filepath.Join(m.symbolDir, name)
		candles := readCandleFile(path)
		if len(candles) > 0 {
			m.candles = candles
			return
		}
	}
}

// readCandleFile reads all candles from a zstd-compressed file and closes it.
func readCandleFile(path string) []indicators.Candle {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	reader, err := zstd.NewReader(file)
	if err != nil {
		return nil
	}
	defer reader.Close()

	var candles []indicators.Candle
	for {
		var c indicators.Candle
		err := c.Deserialize(reader)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			break
		}
		candles = append(candles, c)
	}
	return candles
}

func (m *recordedCandleData) Read(candle *indicators.Candle) error {
	for {
		if m.current < len(m.candles) {
			*candle = m.candles[m.current]
			m.current++
			return nil
		}
		// Current month exhausted, try loading next
		m.loadNextMonth()
		if len(m.candles) == 0 {
			return io.EOF
		}
	}
}

func (m *recordedCandleData) Close() {
	m.candles = nil
	m.files = nil
}
