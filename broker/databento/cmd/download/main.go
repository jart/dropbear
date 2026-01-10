// download fetches Databento historical data and converts to mmap-friendly format.
//
// Usage:
//
//	go run ./broker/databento/cmd/download -symbol GOOG -date 2025-01-06
//	go run ./broker/databento/cmd/download -symbol GOOG -start 2024-12-01 -end 2025-01-01
//
// Output files are stored in ~/databento/{symbol}/{date}.mbo
package main

import (
	"dropbear/broker/databento"
	"dropbear/loggy"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

var (
	flagSymbol  = flag.String("symbol", "", "symbol to download (required)")
	flagDate    = flag.String("date", "", "single date to download (YYYY-MM-DD)")
	flagStart   = flag.String("start", "", "start date for range (YYYY-MM-DD)")
	flagEnd     = flag.String("end", "", "end date for range (YYYY-MM-DD)")
	flagDataset = flag.String("dataset", "XNAS.ITCH", "Databento dataset")
	flagSchema  = flag.String("schema", "mbo", "schema: mbo, mbp-1, mbp-10, trades")
	flagOutDir  = flag.String("outdir", "", "output directory (default: ~/databento)")
	flagDry     = flag.Bool("dry", false, "dry run - show what would be downloaded")
)

func main() {
	flag.Parse()
	loggy.Init()

	if *flagSymbol == "" {
		loggy.Fatalf("missing required -symbol flag")
	}

	// Determine date range
	var dates []time.Time
	if *flagDate != "" {
		d, err := time.Parse("2006-01-02", *flagDate)
		if err != nil {
			loggy.Fatalf("invalid date %q: %v", *flagDate, err)
		}
		dates = append(dates, d)
	} else if *flagStart != "" && *flagEnd != "" {
		start, err := time.Parse("2006-01-02", *flagStart)
		if err != nil {
			loggy.Fatalf("invalid start date %q: %v", *flagStart, err)
		}
		end, err := time.Parse("2006-01-02", *flagEnd)
		if err != nil {
			loggy.Fatalf("invalid end date %q: %v", *flagEnd, err)
		}
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			// Skip weekends
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			dates = append(dates, d)
		}
	} else {
		loggy.Fatalf("must specify -date or both -start and -end")
	}

	// Determine output directory
	outDir := *flagOutDir
	if outDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			loggy.Fatalf("get home dir: %v", err)
		}
		outDir = filepath.Join(home, "databento")
	}

	// Load API key
	apiKey, err := databento.GetKey()
	if err != nil {
		loggy.Fatalf("get API key: %v", err)
	}

	// Download each date
	for _, date := range dates {
		if err := downloadDay(apiKey, *flagSymbol, date, outDir); err != nil {
			log.Printf("error downloading %s %s: %v", *flagSymbol, date.Format("2006-01-02"), err)
		}
	}
}

func downloadDay(apiKey, symbol string, date time.Time, outDir string) error {
	dateStr := date.Format("2006-01-02")

	// Create output directory
	symbolDir := filepath.Join(outDir, symbol)
	if err := os.MkdirAll(symbolDir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	// Check if file already exists
	outPath := filepath.Join(symbolDir, dateStr+".mbo")
	if _, err := os.Stat(outPath); err == nil {
		log.Printf("skip %s %s (already exists)", symbol, dateStr)
		return nil
	}

	if *flagDry {
		log.Printf("would download %s %s", symbol, dateStr)
		return nil
	}

	log.Printf("downloading %s %s ...", symbol, dateStr)

	// Build URL - market hours are 9:30-16:00 ET
	// Use full day to be safe (API will return data within market hours)
	startTime := date.Format("2006-01-02") + "T00:00:00"
	endTime := date.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00"

	url := fmt.Sprintf(
		"https://hist.databento.com/v0/timeseries.get_range?dataset=%s&symbols=%s&schema=%s&start=%s&end=%s&encoding=dbn",
		*flagDataset, symbol, *flagSchema, startTime, endTime,
	)

	// Create request with auth
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.SetBasicAuth(apiKey, "")

	// Execute request
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	// Read response into memory
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Parse DBN header to find where records start
	if len(data) < 8 {
		return fmt.Errorf("response too short: %d bytes", len(data))
	}
	if string(data[:3]) != "DBN" {
		return fmt.Errorf("invalid DBN magic: %q", data[:3])
	}

	metaLen := binary.LittleEndian.Uint32(data[4:8])
	headerSize := 8 + int(metaLen)

	if headerSize >= len(data) {
		log.Printf("skip %s %s (no records - market closed?)", symbol, dateStr)
		return nil
	}

	records := data[headerSize:]
	recordSize := int(unsafe.Sizeof(databento.MboMsg{}))
	numRecords := len(records) / recordSize

	if numRecords == 0 {
		log.Printf("skip %s %s (no records)", symbol, dateStr)
		return nil
	}

	// Write raw records to file
	if err := os.WriteFile(outPath, records, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	log.Printf("wrote %s: %d records (%d bytes)", outPath, numRecords, len(records))
	return nil
}
