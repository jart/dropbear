package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/ds"
	"dropbear/loggy"
	"dropbear/symbol"
)

var (
	flagFeed       = flag.String("feed", "sip", "market data feed (sip, iex, otc, boats)")
	flagWorkers    = flag.Int("workers", 20, "number of parallel download workers")
	flagAdjustment = flag.String("adjustment", "all", "price adjustment (raw, split, dividend, spin-off, all)")
)

var (
	feed       alpaca.DataFeed
	adjustment alpaca.BarAdjustment
	errorCount atomic.Int32
)

const barSize = int(unsafe.Sizeof(ds.Bar{}))

func main() {
	flag.Parse()
	loggy.Init()
	client := alpaca.NewClient()

	// parse flags into typed values
	var err error
	feed, err = alpaca.ParseDataFeed(*flagFeed)
	if err != nil {
		log.Fatalf("invalid feed: %v", err)
	}
	adjustment, err = alpaca.ParseBarAdjustment(*flagAdjustment)
	if err != nil {
		log.Fatalf("invalid adjustment: %v", err)
	}

	// clean up any leftover temp files from interrupted runs
	cleanupTempFiles()

	// get symbols from args, otherwise fetch everything
	var symbolMap = make(map[string]struct{})
	if len(flag.Args()) == 0 {
		log.Println("no symbols specified, fetching the healthiest u.s. equities...")
		err := client.SyncAssets()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to sync assets: %v\n", err)
			os.Exit(1)
		}
		count := 0
		for _, a := range alpaca.Assets {
			if a.IsHealthy() {
				symbolMap[a.Symbol.String()] = struct{}{}
				count++
			}
		}
		log.Printf("found %d healthy u.s. equities", count)
		count = 0
		entries, err := os.ReadDir(ds.EquityMinutesDir())
		if err == nil {
			for _, entry := range entries {
				name := entry.Name()
				sym, err := symbol.Parse(name)
				if err != nil {
					continue
				}
				if alpaca.Assets[sym] == nil {
					continue
				}
				symbolMap[name] = struct{}{}
				count++
			}
		}
		log.Printf("including %d existing symbols", count)
	} else {
		for _, arg := range flag.Args() {
			symbolMap[arg] = struct{}{}
		}
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

	// start workers
	var wg sync.WaitGroup
	for i := 0; i < *flagWorkers; i++ {
		wg.Go(func() {
			for sym := range jobs {
				if stopped.Load() {
					return
				}
				if !downloadSymbol(client, sym, &stopped) {
					errorCount.Add(1)
				}
			}
		})
	}

	// queue symbols
	go func() {
		defer close(jobs)
		for sym := range symbolMap {
			if stopped.Load() {
				return
			}
			jobs <- sym
		}
	}()

	wg.Wait()
	log.Printf("finished with %d errors\n", errorCount.Load())
	os.Exit(min(int(errorCount.Load()), 255))
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

func downloadSymbol(client *alpaca.Client, sym string, stopped *atomic.Bool) bool {
	outPath := filepath.Join(ds.EquityMinutesDir(), sym)
	tmpPath := outPath + ".tmp"

	// read existing data and find where to resume from
	var existingData []byte
	var lastTime clocky.Time
	if data, err := os.ReadFile(outPath); err == nil && len(data) >= barSize {
		existingData = data
		n := len(data) / barSize
		lastBar := (*ds.Bar)(unsafe.Pointer(&data[(n-1)*barSize]))
		lastTime = lastBar.Timestamp
	}

	// determine start time
	var start clocky.Time
	if lastTime != 0 {
		// Resume from 1 minute after last bar
		start = lastTime.Add(clocky.Minute)
	} else {
		// Start from beginning of Alpaca's data
		loc, _ := time.LoadLocation("America/New_York")
		start = clocky.Time(time.Date(2016, 1, 1, 0, 0, 0, 0, loc).UnixMicro())
	}

	// end at now
	end := clocky.RealNow()

	if start >= end {
		log.Printf("%s: already up to date", sym)
		return true
	}

	// download all bars
	bars, err := fetchAll(client, sym, start, end, stopped)
	if err != nil {
		if err != errInterrupted {
			log.Printf("%s: %v", sym, err)
		}
		return false
	}

	if len(bars) == 0 {
		if lastTime == 0 {
			log.Printf("%s: no data available", sym)
		} else {
			log.Printf("%s: no new data", sym)
		}
		return true
	}

	// ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		log.Printf("%s: mkdir failed: %v", sym, err)
		return false
	}

	// write to temp file first (atomic write)
	f, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("%s: create temp failed: %v", sym, err)
		return false
	}

	out := bufio.NewWriter(f)

	// write existing data first
	if len(existingData) > 0 {
		if _, err := out.Write(existingData); err != nil {
			f.Close()
			os.Remove(tmpPath)
			log.Printf("%s: write existing failed: %v", sym, err)
			return false
		}
	}

	// write new bars
	written := 0
	for i := range bars {
		// Skip any bars at or before our last time (safety check)
		if bars[i].Timestamp <= lastTime {
			continue
		}
		barBytes := unsafe.Slice((*byte)(unsafe.Pointer(&bars[i])), barSize)
		if _, err := out.Write(barBytes); err != nil {
			f.Close()
			os.Remove(tmpPath)
			log.Printf("%s: write failed: %v", sym, err)
			return false
		}
		written++
	}

	if err := out.Flush(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		log.Printf("%s: flush failed: %v", sym, err)
		return false
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		log.Printf("%s: sync failed: %v", sym, err)
		return false
	}
	f.Close()

	// atomic rename
	if err := os.Rename(tmpPath, outPath); err != nil {
		os.Remove(tmpPath)
		log.Printf("%s: rename failed: %v", sym, err)
		return false
	}

	if lastTime == 0 {
		log.Printf("%s: saved %d bars", sym, written)
	} else {
		log.Printf("%s: appended %d bars", sym, written)
	}
	return true
}

var errInterrupted = fmt.Errorf("interrupted")

func fetchAll(client *alpaca.Client, sym string, start, end clocky.Time, stopped *atomic.Bool) ([]ds.Bar, error) {
	var bars []ds.Bar
	pageToken := ""
	for {
		if stopped.Load() {
			return nil, errInterrupted
		}
		pageBars, nextToken, err := client.GetBars(symbol.MustParse(sym), clocky.Minute, start, end, feed, adjustment, 10000, false, pageToken)
		if err != nil {
			return nil, err
		}
		bars = append(bars, pageBars...)
		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}
	return bars, nil
}
