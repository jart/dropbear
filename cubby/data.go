package cubby

import (
	"dropbear/clocky"
	"dropbear/indicators"
	"io"
	"os"
	"path/filepath"
	"sort"
)

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
	startYM := start.YearMonthString()
	endYM := end.YearMonthString()

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

// Read copies the next candle into the provided struct.
func (m *recordedCandleData) Read(candle *indicators.Candle) error {
	for {
		if m.reader != nil {
			if c := m.reader.Next(); c != nil {
				*candle = *c
				return nil
			}
		}
		m.loadNextMonth()
		if m.reader == nil {
			return io.EOF
		}
	}
}

// SeekTo binary searches to find first candle >= target.
// Loads the appropriate month file if needed.
func (m *recordedCandleData) SeekTo(target clocky.Time) {
	targetYM := target.YearMonthString()

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

// equityMinutesDir returns the path to the new equity data directory.
func equityMinutesDir() string {
	candidates := []string{
		os.ExpandEnv("$HOME/equitydata/minutes"),
		"/fast/equitydata/minutes",
		"/disk/equitydata/minutes",
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}
