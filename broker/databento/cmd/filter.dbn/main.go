// Command filter.dbn reads a definitions .dbn file to identify 0DTE instrument
// IDs, then filters a data .dbn file (e.g. CBBO-1s) to keep only records for
// those instruments. This dramatically reduces file sizes since most OPRA data
// is for non-0DTE expirations.
//
// Usage:
//
//	go run ./broker/databento/cmd/filter.dbn \
//	    -defs definitions.dbn \
//	    -data cbbo-1s.dbn \
//	    -date 2026-02-27 \
//	    -out cbbo-1s-0dte.dbn
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"
	"unsafe"

	"dropbear/broker/databento"
)

func main() {
	defsPath := flag.String("defs", "", "path to definitions .dbn file (required)")
	dataPath := flag.String("data", "", "path to data .dbn file to filter (required)")
	dateStr := flag.String("date", "", "query date in YYYY-MM-DD format for 0DTE filter (required)")
	outPath := flag.String("out", "", "output .dbn file path (required)")
	flag.Parse()

	if *defsPath == "" || *dataPath == "" || *dateStr == "" || *outPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	date, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		log.Fatalf("bad date %q: %v", *dateStr, err)
	}
	queryDateInt := date.Year()*10000 + int(date.Month())*100 + date.Day()

	// Step 1: Load definitions and find 0DTE instrument IDs.
	ids, err := load0DTEInstrumentIDs(*defsPath, queryDateInt)
	if err != nil {
		log.Fatalf("load definitions: %v", err)
	}
	fmt.Printf("found %d 0DTE instruments in %s\n", len(ids), *defsPath)

	// Step 2: Filter data file to only 0DTE records.
	kept, total, err := filterDBN(*dataPath, *outPath, ids)
	if err != nil {
		log.Fatalf("filter: %v", err)
	}
	fmt.Printf("kept %d of %d records (%.1f%%)\n", kept, total, 100*float64(kept)/float64(total))
}

// load0DTEInstrumentIDs reads a definitions .dbn file and returns the set of
// instrument IDs whose expiration date matches queryDateInt (YYYYMMDD).
func load0DTEInstrumentIDs(path string, queryDateInt int) (map[uint32]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	meta, err := databento.DecodeMetadata(f)
	if err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}

	ids := make(map[uint32]bool)
	for {
		rec, err := databento.DecodeRecord(f)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode record: %w", err)
		}
		rec, err = databento.UpgradeRecord(meta.Version, rec)
		if err != nil {
			return nil, fmt.Errorf("upgrade record: %w", err)
		}
		if len(rec) < int(unsafe.Sizeof(databento.Instrument{})) {
			continue
		}
		inst := (*databento.Instrument)(unsafe.Pointer(&rec[0]))
		expUTC := time.Unix(int64(inst.Expiration)/1e9, int64(inst.Expiration)%1e9).UTC()
		expDateInt := expUTC.Year()*10000 + int(expUTC.Month())*100 + expUTC.Day()
		if expDateInt == queryDateInt {
			ids[inst.Header.InstrumentID] = true
		}
	}
	return ids, nil
}

// filterDBN copies the metadata header from dataPath to outPath, then copies
// only records whose InstrumentID is in the ids set.
func filterDBN(dataPath, outPath string, ids map[uint32]bool) (kept, total int, err error) {
	in, err := os.Open(dataPath)
	if err != nil {
		return 0, 0, err
	}
	defer in.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()

	// Read and copy the prelude (8 bytes).
	var prelude [databento.MetadataPreludeSize]byte
	if _, err := io.ReadFull(in, prelude[:]); err != nil {
		return 0, 0, fmt.Errorf("read prelude: %w", err)
	}
	frameSize := binary.LittleEndian.Uint32(prelude[4:8])
	if _, err := out.Write(prelude[:]); err != nil {
		return 0, 0, fmt.Errorf("write prelude: %w", err)
	}

	// Read and copy the metadata frame.
	frame := make([]byte, frameSize)
	if _, err := io.ReadFull(in, frame); err != nil {
		return 0, 0, fmt.Errorf("read frame: %w", err)
	}
	if _, err := out.Write(frame); err != nil {
		return 0, 0, fmt.Errorf("write frame: %w", err)
	}

	// Filter records.
	for {
		rec, err := databento.DecodeRecord(in)
		if err != nil {
			if err == io.EOF {
				break
			}
			return kept, total, fmt.Errorf("decode record: %w", err)
		}
		total++

		// Extract InstrumentID from the record header (bytes 4-8).
		if len(rec) < 8 {
			continue
		}
		instrumentID := binary.LittleEndian.Uint32(rec[4:8])
		if !ids[instrumentID] {
			continue
		}

		if _, err := out.Write(rec); err != nil {
			return kept, total, fmt.Errorf("write record: %w", err)
		}
		kept++

		if total%10_000_000 == 0 {
			fmt.Printf("  progress: %d records scanned, %d kept\n", total, kept)
		}
	}
	return kept, total, nil
}
