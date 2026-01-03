package cubby

import (
	"dropbear/clocky"
	"dropbear/indicators"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"unsafe"
)

// candleReader provides zero-copy iteration over equity candle files.
// Files are memory-mapped for efficient access.
type candleReader struct {
	file   *os.File
	data   []byte
	offset int
}

// openCandleReader opens an equitydata2 file for reading.
// The file format is raw 48-byte candles with no length prefix.
func openCandleReader(path string) (*candleReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		file.Close()
		return &candleReader{}, nil
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, err
	}
	// Tell the kernel we'll read sequentially
	return &candleReader{
		file: file,
		data: data,
	}, nil
}

// candleSize is the size of a Candle in bytes (6 x int64 = 48 bytes).
const candleSize = 48

// Next returns a pointer to the next candle in the file.
// Returns nil when there are no more candles.
// The returned pointer is only valid while the file is open.
func (r *candleReader) Next() *indicators.Candle {
	if r.offset+candleSize > len(r.data) {
		return nil
	}
	// Zero-copy: cast directly to Candle pointer
	candle := (*indicators.Candle)(unsafe.Pointer(&r.data[r.offset]))
	r.offset += candleSize
	return candle
}

// SeekTo uses binary search to find the first candle >= target time.
func (r *candleReader) SeekTo(target clocky.Time) {
	n := len(r.data) / candleSize
	if n == 0 {
		return
	}
	// Binary search for first candle >= target
	lo, hi := 0, n
	for lo < hi {
		mid := (lo + hi) / 2
		candle := (*indicators.Candle)(unsafe.Pointer(&r.data[mid*candleSize]))
		if candle.Start < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	r.offset = lo * candleSize
}

// Close unmaps the file and closes it.
func (r *candleReader) Close() error {
	if r.data != nil {
		syscall.Munmap(r.data)
		r.data = nil
	}
	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}

// recordedCandleData reads candles from mmap'd monthly files.
// Each month is memory-mapped separately to handle symbols with many years of data.
type recordedCandleData struct {
	symbolDir    string
	files        []string // all monthly files in range
	fileIndex    int      // current position in files
	currentMonth string   // currently loaded month (for SeekTo optimization)
	reader       *candleReader
}

// openRecordedCandleData prepares to read candles for a symbol within the date range.
func openRecordedCandleData(symbol string, start, end clocky.Time) *recordedCandleData {
	base := equityMinutesDir()
	if base == "" {
		return nil
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
	if m.reader == nil {
		return nil
	}

	return m
}

// equityMinutesDir returns the path to the new equity data directory.
func equityMinutesDir() string {
	candidates := []string{
		os.ExpandEnv("$HOME/equitydata2/minutes"),
		"/fast/equitydata2/minutes",
		"/disk/equitydata2/minutes",
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

// loadNextMonth loads the next month's file.
func (m *recordedCandleData) loadNextMonth() {
	if m.reader != nil {
		m.reader.Close()
		m.reader = nil
	}
	m.currentMonth = ""
	for m.fileIndex < len(m.files) {
		name := m.files[m.fileIndex]
		m.fileIndex++
		path := filepath.Join(m.symbolDir, name)
		reader, err := openCandleReader(path)
		if err == nil && reader != nil {
			m.reader = reader
			m.currentMonth = name
			return
		}
	}
}

// Read copies the next candle into the provided struct.
func (m *recordedCandleData) Read(candle *indicators.Candle) error {
	for {
		if m.reader != nil {
			if c := m.reader.Next(); c != nil {
				*candle = *c
				return nil
			}
		}
		// Current month exhausted, try loading next
		m.loadNextMonth()
		if m.reader == nil {
			return io.EOF
		}
	}
}

// SeekTo binary searches to find first candle >= target.
// Loads the appropriate month file if needed.
func (m *recordedCandleData) SeekTo(target clocky.Time) {
	targetYM := target.YearMonth()

	// If already on the target month, just binary search
	if m.currentMonth == targetYM && m.reader != nil {
		m.reader.SeekTo(target)
		return
	}

	// Find the target month in files list
	found := -1
	for i, name := range m.files {
		if name >= targetYM {
			found = i
			break
		}
	}
	if found < 0 {
		// No month >= target, nothing to load
		if m.reader != nil {
			m.reader.Close()
			m.reader = nil
		}
		m.currentMonth = ""
		return
	}

	// Load the target month
	m.fileIndex = found
	m.loadNextMonth()
	if m.reader != nil {
		m.reader.SeekTo(target)
	}
}

// Close releases all resources.
func (m *recordedCandleData) Close() {
	if m.reader != nil {
		m.reader.Close()
		m.reader = nil
	}
	m.files = nil
}
