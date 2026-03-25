// dbndownload fetches OPRA options data from Databento for a specific expiry chain.
//
// Unlike the shell script approach that downloads the entire parent symbol (30+ GB),
// this tool first fetches definitions, filters to instruments expiring on -expiry,
// then launches parallel GetRange requests for each individual instrument, writing
// each to a temp file. After all downloads complete, it k-way merges the temp files
// by TSRecv into the output file.
//
// Usage:
//
//	go run ./broker/databento/cmd/dbndownload -date 2026-03-20 -sym SPXW -schema cmbp-1 -o out.dbn
//	go run ./broker/databento/cmd/dbndownload -date 2026-03-20 -expiry 2026-03-27 -sym BTI -schema cmbp-1 -o out.dbn
//	go run ./broker/databento/cmd/dbndownload -date 2026-03-13 -sym XSP -schema cmbp-1 -j 10 -o out.dbn
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"dropbear/broker/databento"
	"dropbear/clocky"

	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

var (
	dateFlag   = clocky.TimeFlag("date", "", "trading day to download (YYYY-MM-DD)")
	expiryFlag = clocky.TimeFlag("expiry", "", "option expiration date (YYYY-MM-DD, defaults to -date)")
	symFlag    = flag.String("sym", "", "parent symbol (e.g. SPXW, BTI)")
	schemaFlag = flag.String("schema", "cmbp-1", "data schema to download (e.g. cmbp-1, cbbo-1s, trades)")
	outputFlag = flag.String("o", "", "output file path")
	jobsFlag   = flag.Int("j", 50, "max concurrent downloads")
)

func main() {
	flag.Parse()
	if *symFlag == "" || *outputFlag == "" || *dateFlag == 0 {
		fmt.Fprintf(os.Stderr, "usage: dbndownload -date YYYY-MM-DD -sym SPXW -schema cmbp-1 -o out.dbn\n")
		os.Exit(1)
	}

	if *expiryFlag == 0 {
		*expiryFlag = *dateFlag
	}

	schema, err := databento.ParseSchema(*schemaFlag)
	if err != nil {
		log.Fatal(err)
	}

	client := databento.NewHistoricalClient(databento.MustLoadDefaultKey())
	start := *dateFlag
	end := start.Add(clocky.Day)

	// Phase 1: fetch definitions and filter to expiry chain
	log.Println("fetching definitions for", *symFlag)
	instruments := fetchDefinitions(client, start, end)
	log.Printf("found %d instruments expiring on %s", len(instruments), expiryFlag.Format("2006-01-02"))

	// Phase 2: download each instrument to a temp file
	tmpDir, err := os.MkdirTemp("", "dbndownload-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	log.Printf("downloading %s for %d instruments (concurrency %d)", *schemaFlag, len(instruments), *jobsFlag)
	type downloadResult struct {
		path   string
		rawSym string
		count  int
	}
	results := make(chan downloadResult, len(instruments))
	sem := make(chan struct{}, *jobsFlag)
	var wg sync.WaitGroup

	for i, inst := range instruments {
		rawSym := inst.GetRawSymbol()
		tmpPath := filepath.Join(tmpDir, fmt.Sprintf("%06d.dbn", i))
		wg.Add(1)
		go func(rawSym, tmpPath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			count := downloadInstrument(client, schema, rawSym, start, end, tmpPath)
			if count >= 0 {
				results <- downloadResult{path: tmpPath, rawSym: rawSym, count: count}
			}
		}(rawSym, tmpPath)
	}

	// Wait for all downloads, then close results channel
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var files []downloadResult
	for r := range results {
		log.Printf("  %s: %d records", r.rawSym, r.count)
		files = append(files, r)
	}
	log.Printf("downloaded %d instrument files", len(files))

	// Phase 3: write output file
	outFile, err := os.Create(*outputFlag)
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()
	out := bufio.NewWriterSize(outFile, 4*1024*1024)

	// Write metadata header
	meta := databento.Metadata{
		Version:       2,
		Dataset:       "OPRA.PILLAR",
		Schema:        schema,
		Start:         start,
		End:           end,
		STypeIn:       databento.STypeRawSymbol,
		STypeOut:      databento.STypeInstrumentID,
		SymbolCstrLen: databento.SymbolCstrLen,
	}
	if _, err := out.Write(meta.Encode()); err != nil {
		log.Fatal("write metadata:", err)
	}

	// Write instrument definitions
	for _, inst := range instruments {
		if _, err := out.Write(inst.Encode()); err != nil {
			log.Fatal("write definition:", err)
		}
	}

	// Phase 4: k-way merge temp files by TSRecv
	log.Println("merging records by timestamp")
	readers := make([]*databento.FileReader, len(files))
	for i, f := range files {
		r, err := databento.OpenFileReader(f.path)
		if err != nil {
			log.Fatalf("open temp file %s: %v", f.path, err)
		}
		defer r.Close()
		readers[i] = r
	}

	// Seed the heap with the first record from each file
	heap := binaryheap.NewWith(compareHeapItems)
	for i, r := range readers {
		rec, err := r.Read()
		if err == io.EOF {
			continue
		}
		if err != nil {
			log.Fatalf("read temp file: %v", err)
		}
		heap.Push(&heapItem{
			tsRecv: getTSRecv(rec),
			dbn:    rec.(databento.DBN),
			idx:    i,
		})
	}

	written := 0
	for heap.Size() > 0 {
		item, _ := heap.Pop()
		if _, err := out.Write(item.dbn.Encode()); err != nil {
			log.Fatal("write record:", err)
		}
		written++
		if written%100000 == 0 {
			log.Printf("  %d records merged", written)
		}
		// Pull next record from the same file
		rec, err := readers[item.idx].Read()
		if err == io.EOF {
			continue
		}
		if err != nil {
			log.Fatalf("read temp file: %v", err)
		}
		heap.Push(&heapItem{
			tsRecv: getTSRecv(rec),
			dbn:    rec.(databento.DBN),
			idx:    item.idx,
		})
	}

	if err := out.Flush(); err != nil {
		log.Fatal("flush:", err)
	}
	log.Printf("done: %d records written to %s", written, *outputFlag)
}

// downloadInstrument fetches a single instrument's data and writes raw records
// to a temp file. Returns the record count, or -1 on error.
func downloadInstrument(client *databento.HistoricalClient, schema databento.Schema, rawSym string, start, end clocky.Time, path string) int {
	resp, err := client.GetRange(databento.GetRangeParams{
		Dataset: "OPRA.PILLAR",
		Schema:  schema,
		STypeIn: databento.STypeRawSymbol,
		Symbols: []string{rawSym},
		Start:   start,
		End:     end,
	})
	if err != nil {
		log.Printf("error fetching %s: %v", rawSym, err)
		return -1
	}
	defer resp.Close()

	// create temp file and buffered writer
	f, err := os.Create(path)
	if err != nil {
		log.Printf("error creating temp file for %s: %v", rawSym, err)
		return -1
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1024*1024)

	// Write metadata so FileReader can open it later
	if _, err := w.Write(resp.Metadata.Encode()); err != nil {
		log.Printf("error writing metadata for %s: %v", rawSym, err)
		return -1
	}

	count := 0
	for {
		rec, err := resp.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("error reading %s: %v", rawSym, err)
			return -1
		}
		dbn := rec.(databento.DBN)
		if _, err := w.Write(dbn.Encode()); err != nil {
			log.Printf("error writing %s: %v", rawSym, err)
			return -1
		}
		count++
	}

	if err := w.Flush(); err != nil {
		log.Printf("error flushing %s: %v", rawSym, err)
		return -1
	}
	return count
}

func fetchDefinitions(client *databento.HistoricalClient, start, end clocky.Time) []*databento.Instrument {
	resp, err := client.GetRange(databento.GetRangeParams{
		Dataset: "OPRA.PILLAR",
		Schema:  databento.SchemaDefinition,
		STypeIn: databento.STypeParent,
		Symbols: []string{*symFlag + ".OPT"},
		Start:   start,
		End:     end,
	})
	if err != nil {
		log.Fatal("fetch definitions:", err)
	}
	defer resp.Close()

	wantYear, wantMonth, wantDay := expiryFlag.Date()
	var instruments []*databento.Instrument
	for {
		rec, err := resp.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal("read definition:", err)
		}
		inst, ok := rec.(*databento.Instrument)
		if !ok {
			continue
		}
		year, timeMonth, day := inst.Expiration.In(clocky.UTC).Date()
		month := clocky.Month(timeMonth)
		if year == wantYear && month == wantMonth && day == wantDay {
			instruments = append(instruments, inst)
		}
	}
	if len(instruments) == 0 {
		log.Fatal("no instruments found expiring on ", expiryFlag.Format("2006-01-02"))
	}
	return instruments
}

// getTSRecv extracts TSRecv from any record type.
func getTSRecv(rec any) clocky.Time {
	switch r := rec.(type) {
	case *databento.CMBP1:
		return r.TSRecv
	case *databento.CBBO:
		return r.TSRecv
	case *databento.MBP1:
		return r.TSRecv
	case *databento.MBP10:
		return r.TSRecv
	case *databento.Instrument:
		return r.TSRecv
	case *databento.StatusMsg:
		return r.TSRecv
	default:
		log.Fatalf("getTSRecv: unsupported record type %T", rec)
		return 0
	}
}

type heapItem struct {
	tsRecv clocky.Time
	dbn    databento.DBN
	idx    int
}

func compareHeapItems(a, b *heapItem) int {
	if a.tsRecv.Before(b.tsRecv) {
		return -1
	}
	if a.tsRecv.After(b.tsRecv) {
		return +1
	}
	return 0
}
