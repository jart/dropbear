// Command dump.dbn reads a .dbn file and prints its contents. Handles any
// record type (definitions, CBBO, CMBP1, etc.) and formats fields for
// human readability.
//
// Usage:
//
//	go run ./broker/databento/cmd/dump.dbn ~/databento/SPX/2026-02-27.definition.dbn
//	go run ./broker/databento/cmd/dump.dbn -n 20 ~/databento/SPX/2026-02-27.0dte.cbbo-1s.dbn
//	go run ./broker/databento/cmd/dump.dbn -n 10 ~/databento/SPX/2026-02-27.cmbp1.dbn
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"unsafe"

	"dropbear/broker/databento"
)

func main() {
	maxRecords := flag.Int("n", 0, "max records to print (0 = all)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: dump.dbn [-n N] <file.dbn>\n")
		os.Exit(1)
	}
	path := flag.Arg(0)

	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	meta, err := databento.DecodeMetadata(f)
	if err != nil {
		log.Fatalf("decode metadata: %v", err)
	}
	fmt.Printf("%#v\n", meta)

	// Count records by type.
	counts := make(map[databento.RType]int)
	printed := 0
	total := 0
	for {
		rec, err := databento.DecodeRecord(f)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("record %d: %v", total, err)
		}
		rec, err = databento.UpgradeRecord(meta.Version, rec)
		if err != nil {
			log.Fatalf("record %d upgrade: %v", total, err)
		}
		total++

		rtype := databento.RType(rec[1])
		counts[rtype]++

		if *maxRecords > 0 && printed >= *maxRecords {
			continue // keep counting but stop printing
		}
		printRecord(rec, rtype, total-1)
		printed++
	}

	fmt.Printf("\n--- summary ---\n")
	fmt.Printf("total records: %d\n", total)
	for rt, n := range counts {
		fmt.Printf("  %s: %d\n", rt, n)
	}
}

func printRecord(rec []byte, rtype databento.RType, index int) {
	switch rtype {
	case databento.RTypeInstrumentDef:
		if len(rec) >= int(unsafe.Sizeof(databento.Instrument{})) {
			inst := (*databento.Instrument)(unsafe.Pointer(&rec[0]))
			fmt.Printf("%#v\n", inst)
		} else {
			fmt.Printf("\n[%d] InstrumentDef: too small (%d bytes)\n", index, len(rec))
		}
	case databento.RTypeCMBP1S, databento.RTypeCMBP1M:
		if len(rec) >= int(unsafe.Sizeof(databento.CBBO{})) {
			cbbo := (*databento.CBBO)(unsafe.Pointer(&rec[0]))
			fmt.Printf("%#v\n", cbbo)
		}
	case databento.RTypeCMBP1, databento.RTypeTCBBO:
		if len(rec) >= int(unsafe.Sizeof(databento.CMBP1{})) {
			cmbp := (*databento.CMBP1)(unsafe.Pointer(&rec[0]))
			fmt.Printf("%#v\n", cmbp)
		}
	default:
		hdr := (*databento.RecordHeader)(unsafe.Pointer(&rec[0]))
		fmt.Printf("%s  iid=%-10d  ts=%s  (%d bytes)\n",
			rtype, hdr.InstrumentID, hdr.TSEvent, len(rec))
	}
}
