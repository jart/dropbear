package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
)

var (
	flagFeed       = flag.String("feed", "sip", "market data feed (sip, iex)")
	flagWorkers    = flag.Int("workers", 20, "number of parallel download workers")
	flagAdjustment = flag.String("adjustment", "all", "price adjustment (raw, split, dividend, all)")
)

const (
	dataURL    = "https://data.alpaca.markets"
	candleSize = 48
)

func main() {
	flag.Parse()
	loggy.Init()

	// Clean up any leftover temp files from interrupted runs
	cleanupTempFiles()

	// get symbols from args, otherwise fetch everything
	symbols := flag.Args()
	if len(symbols) == 0 {
		log.Println("No symbols specified, fetching all tradable US equities...")
		client := alpaca.NewClient()
		err := client.SyncAssets()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to sync assets: %v\n", err)
			os.Exit(1)
		}
		for _, a := range alpaca.Assets {
			if a.Class == alpaca.AssetClassUSEquity &&
				a.Status == alpaca.AssetStatusActive &&
				a.Tradable == ds.True &&
				a.PTPNoException == ds.False &&
				a.PTPWithException == ds.False {
				symbols = append(symbols, a.Symbol)
			}
		}
		log.Printf("Found %d tradable US equities", len(symbols))
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	jobs := make(chan string, 100)
	var stopped atomic.Bool

	go func() {
		<-stop
		log.Println("interrupted, finishing current downloads...")
		stopped.Store(true)
	}()

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < *flagWorkers; i++ {
		wg.Go(func() {
			client := alpaca.NewClient()
			for symbol := range jobs {
				if stopped.Load() {
					return
				}
				downloadSymbol(client, symbol, &stopped)
			}
		})
	}

	// Queue symbols
	go func() {
		defer close(jobs)
		for _, symbol := range symbols {
			if stopped.Load() {
				return
			}
			jobs <- symbol
		}
	}()

	wg.Wait()
	log.Println("done")
}

func cleanupTempFiles() {
	dir := ds.EquityMinutesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // directory might not exist yet
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			tmpPath := filepath.Join(dir, entry.Name())
			if err := os.Remove(tmpPath); err == nil {
				log.Printf("cleaned up: %s", entry.Name())
			}
		}
	}
}

func downloadSymbol(client *alpaca.Client, symbol string, stopped *atomic.Bool) {
	outPath := filepath.Join(ds.EquityMinutesDir(), symbol)
	tmpPath := outPath + ".tmp"

	// Read existing data and find where to resume from
	var existingData []byte
	var lastTime clocky.Time
	if data, err := os.ReadFile(outPath); err == nil && len(data) >= candleSize {
		existingData = data
		n := len(data) / candleSize
		off := (n - 1) * candleSize
		lastTime = clocky.Time(int64(data[off]) |
			int64(data[off+1])<<8 |
			int64(data[off+2])<<16 |
			int64(data[off+3])<<24 |
			int64(data[off+4])<<32 |
			int64(data[off+5])<<40 |
			int64(data[off+6])<<48 |
			int64(data[off+7])<<56)
	}

	// Determine start time
	loc, _ := time.LoadLocation("America/New_York")
	var start time.Time
	if lastTime != 0 {
		// Resume from 1 minute after last candle
		start = time.UnixMicro(int64(lastTime)).Add(time.Minute)
	} else {
		// Start from beginning of Alpaca's data
		start = time.Date(2016, 1, 1, 0, 0, 0, 0, loc)
	}

	// End at now
	end := time.Now().In(loc)

	if !start.Before(end) {
		log.Printf("%s: already up to date", symbol)
		return
	}

	// Download all candles
	candles, err := fetchAll(client, symbol, start, end, stopped)
	if err != nil {
		if err != errInterrupted {
			log.Printf("%s: %v", symbol, err)
		}
		return
	}

	if len(candles) == 0 {
		if lastTime == 0 {
			log.Printf("%s: no data available", symbol)
		} else {
			log.Printf("%s: no new data", symbol)
		}
		return
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		log.Printf("%s: mkdir failed: %v", symbol, err)
		return
	}

	// Write to temp file first (atomic write)
	f, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("%s: create temp failed: %v", symbol, err)
		return
	}

	out := bufio.NewWriter(f)

	// Write existing data first
	if len(existingData) > 0 {
		if _, err := out.Write(existingData); err != nil {
			f.Close()
			os.Remove(tmpPath)
			log.Printf("%s: write existing failed: %v", symbol, err)
			return
		}
	}

	// Write new candles
	var buf [candleSize]byte
	written := 0
	for i := range candles {
		// Skip any candles at or before our last time (safety check)
		if candles[i].Start <= lastTime {
			continue
		}
		if _, err := out.Write(candles[i].Encode(buf[:0])); err != nil {
			f.Close()
			os.Remove(tmpPath)
			log.Printf("%s: write failed: %v", symbol, err)
			return
		}
		written++
	}

	if err := out.Flush(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		log.Printf("%s: flush failed: %v", symbol, err)
		return
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		log.Printf("%s: sync failed: %v", symbol, err)
		return
	}
	f.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, outPath); err != nil {
		os.Remove(tmpPath)
		log.Printf("%s: rename failed: %v", symbol, err)
		return
	}

	if lastTime == 0 {
		log.Printf("%s: saved %d candles", symbol, written)
	} else {
		log.Printf("%s: appended %d candles", symbol, written)
	}
}

type barsResponse struct {
	Bars      []barData `json:"bars"`
	NextToken string    `json:"next_page_token"`
}

type barData struct {
	Timestamp  string      `json:"t"`
	Open       json.Number `json:"o"`
	High       json.Number `json:"h"`
	Low        json.Number `json:"l"`
	Close      json.Number `json:"c"`
	Volume     json.Number `json:"v"`
	TradeCount int         `json:"n"`
	VWAP       json.Number `json:"vw"`
}

var errInterrupted = fmt.Errorf("interrupted")

func fetchAll(client *alpaca.Client, symbol string, start, end time.Time, stopped *atomic.Bool) ([]indicators.Candle, error) {
	var candles []indicators.Candle
	pageToken := ""

	for {
		if stopped.Load() {
			return nil, errInterrupted
		}

		url := fmt.Sprintf("%s/v2/stocks/%s/bars?timeframe=1Min&start=%s&end=%s&feed=%s&adjustment=%s&limit=10000",
			dataURL, symbol,
			start.UTC().Format(time.RFC3339),
			end.UTC().Format(time.RFC3339),
			*flagFeed,
			*flagAdjustment)
		if pageToken != "" {
			url += "&page_token=" + pageToken
		}

		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}

		var data barsResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode: %w", err)
		}
		resp.Body.Close()

		for _, bar := range data.Bars {
			t, err := clocky.ParseTime(bar.Timestamp)
			if err != nil {
				return nil, fmt.Errorf("parse timestamp %q: %w", bar.Timestamp, err)
			}
			candles = append(candles, indicators.Candle{
				Start:  t,
				Open:   decimal.Parse(bar.Open.String()),
				High:   decimal.Parse(bar.High.String()),
				Low:    decimal.Parse(bar.Low.String()),
				Close:  decimal.Parse(bar.Close.String()),
				Volume: decimal.Parse(bar.Volume.String()),
			})
		}

		if data.NextToken == "" {
			break
		}
		pageToken = data.NextToken
	}

	return candles, nil
}
