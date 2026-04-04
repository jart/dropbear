// dbninject downloads data for an additional symbol and merges it into an
// existing DBN file. This is useful for injecting underlying equity or futures
// quotes into an OPRA options file, so backtesting can use real underlying
// prices instead of inferring them from the options chain.
//
// Usage:
//
//	go run ./broker/databento/cmd/dbninject -file ~/databento/NVDA/2026-03-25.dbn -dataset XNAS.ITCH -sym NVDA -schema mbp-1
//	go run ./broker/databento/cmd/dbninject -file ~/databento/SPXW/2026-03-25.dbn -dataset GLBX.MDP3 -sym ES.FUT -schema mbp-1 -stype continuous
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"dropbear/broker/databento"
	"dropbear/clocky"

	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

var (
	fileFlag    = flag.String("file", "", "path to existing DBN file")
	datasetFlag = flag.String("dataset", "", "dataset for the new symbol (e.g. XNAS.ITCH, GLBX.MDP3)")
	symFlag     = flag.String("sym", "", "symbol to inject (e.g. NVDA, ES.FUT)")
	schemaFlag  = flag.String("schema", "mbp-1", "schema to download (e.g. mbp-1)")
	stypeFlag   = flag.String("stype", "raw_symbol", "input symbology type (e.g. raw_symbol, continuous, parent)")
)

func main() {
	flag.Parse()
	if *fileFlag == "" || *datasetFlag == "" || *symFlag == "" {
		fmt.Fprintf(os.Stderr, "usage: dbninject -file FILE -dataset DATASET -sym SYMBOL [-schema mbp-1] [-stype raw_symbol]\n")
		os.Exit(1)
	}

	schema, err := databento.ParseSchema(*schemaFlag)
	if err != nil {
		log.Fatal(err)
	}

	stype, err := databento.ParseSType(*stypeFlag)
	if err != nil {
		log.Fatal(err)
	}

	// Phase 1: open existing file and read instrument definitions
	log.Printf("opening %s", *fileFlag)
	src, err := databento.OpenFileReader(*fileFlag)
	if err != nil {
		log.Fatal(err)
	}
	defer src.Close()

	start := src.Metadata.Start
	end := src.Metadata.End
	log.Printf("date range: %s to %s", start.Format("2006-01-02"), end.Format("2006-01-02"))

	// Read instrument definitions (they come first in the file)
	var instruments []*databento.Instrument
	var firstDataRecord databento.DBN
	for {
		rec, err := src.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal("read source:", err)
		}
		if inst, ok := rec.(*databento.Instrument); ok {
			instruments = append(instruments, inst)
		} else {
			firstDataRecord = rec.(databento.DBN)
			break
		}
	}
	log.Printf("read %d instruments from source", len(instruments))

	// Phase 2: download definition + data for the injected symbol
	client := databento.NewHistoricalClient(databento.MustLoadDefaultKey())

	// For continuous stype (e.g. ES.FUT), the historical API doesn't support
	// it. We fetch definitions with parent stype, find the front-month contract
	// (earliest expiry on or after the trading date), then fetch data for that
	// specific contract by raw_symbol.
	dataStype := stype
	dataSym := *symFlag
	log.Printf("fetching definitions for %s from %s", *symFlag, *datasetFlag)
	defStype := stype
	if defStype == databento.STypeContinuous {
		defStype = databento.STypeParent
	}
	defResp, err := client.GetRange(databento.GetRangeParams{
		Dataset: *datasetFlag,
		Schema:  databento.SchemaDefinition,
		STypeIn: defStype,
		Symbols: []string{*symFlag},
		Start:   start,
		End:     end,
	})
	if err != nil {
		log.Fatal("fetch definition:", err)
	}
	var allInstruments []*databento.Instrument
	for {
		rec, err := defResp.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal("read definition:", err)
		}
		if inst, ok := rec.(*databento.Instrument); ok {
			allInstruments = append(allInstruments, inst)
		}
	}
	defResp.Close()

	// For continuous stype, find front-month contract and switch to raw_symbol
	var newInstruments []*databento.Instrument
	if stype == databento.STypeContinuous {
		var frontMonth *databento.Instrument
		for _, inst := range allInstruments {
			exp := inst.ExpirationWeird
			if exp == 0 || exp.Before(start) {
				continue
			}
			if strings.Contains(inst.GetRawSymbol(), "-") {
				continue // skip spreads (e.g. ESM5-ESH6)
			}
			if frontMonth == nil || exp.Before(frontMonth.ExpirationWeird) {
				frontMonth = inst
			}
		}
		if frontMonth == nil {
			log.Fatal("no front-month contract found for ", *symFlag)
		}
		dataSym = frontMonth.GetRawSymbol()
		dataStype = databento.STypeRawSymbol
		newInstruments = []*databento.Instrument{frontMonth}
		log.Printf("front-month contract: %s (expires %s)", dataSym,
			frontMonth.ExpirationWeird.Format("2006-01-02"))
	} else {
		newInstruments = allInstruments
	}
	log.Printf("got %d instrument definitions for %s", len(newInstruments), dataSym)
	if len(newInstruments) == 0 {
		log.Fatal("no instruments found for ", *symFlag)
	}

	log.Printf("downloading %s data for %s", *schemaFlag, dataSym)
	dataResp, err := client.GetRange(databento.GetRangeParams{
		Dataset: *datasetFlag,
		Schema:  schema,
		STypeIn: dataStype,
		Symbols: []string{dataSym},
		Start:   start,
		End:     end,
	})
	if err != nil {
		log.Fatal("fetch data:", err)
	}
	defer dataResp.Close()

	// Phase 3: write output file
	tmpPath := *fileFlag + ".tmp"
	outFile, err := os.Create(tmpPath)
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()
	out := bufio.NewWriterSize(outFile, 4*1024*1024)

	// Write original metadata
	if _, err := out.Write(src.Metadata.Encode()); err != nil {
		log.Fatal("write metadata:", err)
	}

	// Write original instrument definitions
	for _, inst := range instruments {
		if _, err := out.Write(inst.Encode()); err != nil {
			log.Fatal("write instrument:", err)
		}
	}

	// Write new instrument definitions
	for _, inst := range newInstruments {
		if _, err := out.Write(inst.Encode()); err != nil {
			log.Fatal("write new instrument:", err)
		}
	}

	// Phase 4: merge old data records with downloaded records by TSRecv
	log.Println("merging records by timestamp")
	heap := binaryheap.NewWith(compareHeapItems)

	// Seed heap with first old record (already read above)
	if firstDataRecord != nil {
		heap.Push(&heapItem{
			tsRecv: firstDataRecord.GetTSRecv(),
			dbn:    firstDataRecord,
			source: sourceOld,
		})
	}

	// Seed heap with first new record
	newRec, newErr := dataResp.Read()
	if newErr == nil {
		dbn := newRec.(databento.DBN)
		heap.Push(&heapItem{
			tsRecv: dbn.GetTSRecv(),
			dbn:    dbn,
			source: sourceNew,
		})
	} else if newErr != io.EOF {
		log.Fatal("read new data:", newErr)
	}

	written := 0
	for heap.Size() > 0 {
		item, _ := heap.Pop()
		if _, err := out.Write(item.dbn.Encode()); err != nil {
			log.Fatal("write record:", err)
		}
		written++
		if written%500000 == 0 {
			log.Printf("  %d records merged", written)
		}

		// Refill from the same source
		switch item.source {
		case sourceOld:
			rec, err := src.Read()
			if err == nil {
				dbn := rec.(databento.DBN)
				heap.Push(&heapItem{
					tsRecv: dbn.GetTSRecv(),
					dbn:    dbn,
					source: sourceOld,
				})
			} else if err != io.EOF {
				log.Fatal("read source:", err)
			}
		case sourceNew:
			rec, err := dataResp.Read()
			if err == nil {
				dbn := rec.(databento.DBN)
				heap.Push(&heapItem{
					tsRecv: dbn.GetTSRecv(),
					dbn:    dbn,
					source: sourceNew,
				})
			} else if err != io.EOF {
				log.Fatal("read new data:", err)
			}
		}
	}

	if err := out.Flush(); err != nil {
		log.Fatal("flush:", err)
	}
	if err := outFile.Close(); err != nil {
		log.Fatal("close:", err)
	}
	if err := os.Rename(tmpPath, *fileFlag); err != nil {
		log.Fatal("rename:", err)
	}
	log.Printf("done: %d+%d instruments, %d records written to %s",
		len(instruments), len(newInstruments), written, *fileFlag)
}

const (
	sourceOld = 0
	sourceNew = 1
)

type heapItem struct {
	tsRecv clocky.Time
	dbn    databento.DBN
	source int
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
