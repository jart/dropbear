// Command refresh_recent redownloads November and December 2025 candles
// for all existing symbols to fix incomplete data.
//
// Usage:
//
//	go run ./cmd/trash/refresh_recent.go
package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/alpaca"
	"dropbear/indicators"
	"dropbear/loggy"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
)

var (
	flagWorkers = flag.Int("workers", 8, "number of parallel workers")
	flagFeed    = flag.String("feed", "sip", "data feed (sip or iex)")
)

const dataURL = "https://data.alpaca.markets"

func main() {
	flag.Parse()
	loggy.Init()

	// Find all existing symbols
	baseDir := ds.EquityMinutesDir()
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		loggy.Fatalf("failed to read %s: %v", baseDir, err)
	}

	var symbols []string
	for _, e := range entries {
		if e.IsDir() {
			symbols = append(symbols, e.Name())
		}
	}
	log.Printf("Found %d symbols to refresh", len(symbols))

	// Months to refresh: November 2025 and December 2025
	loc, _ := time.LoadLocation("America/New_York")
	months := []time.Time{
		time.Date(2025, 11, 1, 0, 0, 0, 0, loc),
		time.Date(2025, 12, 1, 0, 0, 0, 0, loc),
	}

	// Delete existing November files (they're incomplete)
	log.Println("Deleting incomplete November 2025 files...")
	deleted := 0
	for _, symbol := range symbols {
		novPath := filepath.Join(baseDir, symbol, "2025-11")
		if _, err := os.Stat(novPath); err == nil {
			os.Remove(novPath)
			deleted++
		}
	}
	log.Printf("Deleted %d November files", deleted)

	// Queue jobs
	type job struct {
		symbol string
		month  time.Time
	}
	jobs := make(chan job, 1000)

	go func() {
		defer close(jobs)
		for _, month := range months {
			for _, symbol := range symbols {
				// Skip if already exists
				outPath := filepath.Join(baseDir, symbol, month.Format("2006-01"))
				if _, err := os.Stat(outPath); err == nil {
					continue
				}
				jobs <- job{symbol: symbol, month: month}
			}
		}
	}()

	// Workers
	var wg sync.WaitGroup
	var completed int64
	var mu sync.Mutex
	for i := 0; i < *flagWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				downloadMonth(j.symbol, j.month)
				mu.Lock()
				completed++
				if completed%50 == 0 {
					log.Printf("Completed %d downloads", completed)
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	log.Printf("Done! Completed %d downloads", completed)
}

type barsResponse struct {
	Bars      []barData `json:"bars"`
	NextToken string    `json:"next_page_token"`
}

type barData struct {
	Timestamp  string  `json:"t"`
	Open       float64 `json:"o"`
	High       float64 `json:"h"`
	Low        float64 `json:"l"`
	Close      float64 `json:"c"`
	Volume     float64 `json:"v"`
	TradeCount int     `json:"n"`
	VWAP       float64 `json:"vw"`
}

func downloadMonth(symbol string, month time.Time) {
	baseDir := ds.EquityMinutesDir()
	outDir := filepath.Join(baseDir, symbol)
	outPath := filepath.Join(outDir, month.Format("2006-01"))

	candles, err := fetchMonth(symbol, month)
	if err != nil {
		log.Printf("%s %s: error: %v", symbol, month.Format("2006-01"), err)
		return
	}
	if len(candles) == 0 {
		return
	}

	os.MkdirAll(outDir, 0755)
	tmp := outPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		log.Printf("%s: create error: %v", symbol, err)
		return
	}

	w, _ := zstd.NewWriter(f)
	for i := range candles {
		candles[i].Serialize(w)
	}
	w.Close()
	f.Close()

	os.Rename(tmp, outPath)
	log.Printf("%s %s: saved %d candles", symbol, month.Format("2006-01"), len(candles))
}

func fetchMonth(symbol string, month time.Time) ([]indicators.Candle, error) {
	var candles []indicators.Candle
	pageToken := ""

	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())
	end := start.AddDate(0, 1, 0)

	for {
		url := fmt.Sprintf("%s/v2/stocks/%s/bars?timeframe=1Min&start=%s&end=%s&feed=%s&adjustment=split&limit=10000",
			dataURL, symbol,
			start.UTC().Format(time.RFC3339),
			end.UTC().Format(time.RFC3339),
			*flagFeed)

		if pageToken != "" {
			url += "&page_token=" + pageToken
		}

		resp, err := dataRequest(url)
		if err != nil {
			return nil, err
		}

		for _, bar := range resp.Bars {
			ts, _ := time.Parse(time.RFC3339, bar.Timestamp)
			candles = append(candles, indicators.Candle{
				Start:  clocky.Time(ts.UnixMicro()),
				Open:   decimal.FromFloat64(bar.Open),
				High:   decimal.FromFloat64(bar.High),
				Low:    decimal.FromFloat64(bar.Low),
				Close:  decimal.FromFloat64(bar.Close),
				Volume: decimal.FromFloat64(bar.Volume),
			})
		}

		if resp.NextToken == "" {
			break
		}
		pageToken = resp.NextToken
	}

	return candles, nil
}

func dataRequest(url string) (*barsResponse, error) {
	req, _ := http.NewRequest("GET", url, nil)
	key, secret := alpaca.GetKey()
	req.Header.Set("APCA-API-KEY-ID", key)
	req.Header.Set("APCA-API-SECRET-KEY", secret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}

	var result barsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}
