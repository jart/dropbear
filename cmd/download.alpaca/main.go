package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
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

const dataURL = "https://data.alpaca.markets"

func main() {
	flag.Parse()
	loggy.Init()

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

	// Create work queue
	type job struct {
		symbol string
		month  time.Time
	}
	jobs := make(chan job, 1000)
	var stopped atomic.Bool
	var exhausted sync.Map // symbols that have no more historical data

	// Handle interrupt
	go func() {
		<-stop
		log.Println("interrupted, finishing current downloads...")
		stopped.Store(true)
		// Don't close jobs here - let producer close it when it detects stopped
	}()

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < *flagWorkers; i++ {
		wg.Go(func() {
			client := alpaca.NewClient()
			for j := range jobs {
				if stopped.Load() {
					return
				}
				if !downloadMonth(client, j.symbol, j.month) {
					exhausted.Store(j.symbol, true)
				}
			}
		})
	}

	// Queue up work - start from current month (for cron freshness)
	loc, _ := time.LoadLocation("America/New_York")
	now := time.Now().In(loc)
	startMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	// Enqueue all symbol/month combinations
	go func() {
		defer close(jobs)
		month := startMonth
		// Alpaca has data back to ~2016, don't go further
		floor := time.Date(2016, 1, 1, 0, 0, 0, 0, loc)
		for month.After(floor) || month.Equal(floor) {
			if stopped.Load() {
				return
			}
			for _, symbol := range symbols {
				if stopped.Load() {
					return
				}
				// Skip symbols that have no more historical data
				if _, done := exhausted.Load(symbol); done {
					continue
				}
				// Check if already exists
				outPath := filepath.Join(
					ds.EquityMinutesDir(),
					symbol,
					month.Format("2006-01"),
				)
				if info, err := os.Stat(outPath); err == nil {
					// For current month, redownload if file is stale (modified before today)
					if month.Equal(startMonth) && info.ModTime().Before(today) {
						os.Remove(outPath) // delete stale file
					} else {
						continue // already have it and it's fresh
					}
				}
				jobs <- job{symbol: symbol, month: month}
			}
			month = month.AddDate(0, -1, 0)
		}
	}()

	wg.Wait()
	log.Println("done")
}

func downloadMonth(client *alpaca.Client, symbol string, month time.Time) bool {
	// Build output path: ~/equitydata/minutes/SYMBOL/YYYY-MM
	outDir := filepath.Join(ds.EquityMinutesDir(), symbol)
	outPath := filepath.Join(outDir, month.Format("2006-01"))

	// Download the month's data
	candles, err := fetchMonth(client, symbol, month)
	if err != nil {
		log.Printf("%s %s: %v", symbol, month.Format("2006-01"), err)
		return true // error doesn't mean exhausted, might be transient
	}

	if len(candles) == 0 {
		log.Printf("%s %s: no data (exhausted)", symbol, month.Format("2006-01"))
		return false // no more historical data for this symbol
	}

	// Write to temp file first
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Printf("%s %s: mkdir failed: %v", symbol, month.Format("2006-01"), err)
		return true
	}

	tmpPath := outPath + ".tmp"
	if err := writeCandles(tmpPath, candles); err != nil {
		os.Remove(tmpPath)
		log.Printf("%s %s: write failed: %v", symbol, month.Format("2006-01"), err)
		return true
	}

	// Atomic rename
	if err := os.Rename(tmpPath, outPath); err != nil {
		os.Remove(tmpPath)
		log.Printf("%s %s: rename failed: %v", symbol, month.Format("2006-01"), err)
		return true
	}

	log.Printf("%s %s: saved %d candles", symbol, month.Format("2006-01"), len(candles))
	return true
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

func fetchMonth(client *alpaca.Client, symbol string, month time.Time) ([]indicators.Candle, error) {
	var candles []indicators.Candle
	pageToken := ""

	// First day of month at market open to first day of next month
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())
	end := start.AddDate(0, 1, 0)

	for {
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

func writeCandles(path string, candles []indicators.Candle) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// Write raw 48-byte candles (no compression, no length prefix)
	buf := make([]byte, 0, len(candles)*48)
	for i := range candles {
		buf = candles[i].Encode(buf)
	}
	_, err = f.Write(buf)
	return err
}
