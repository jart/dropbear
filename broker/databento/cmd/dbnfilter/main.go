// dbnfilter is a tool for making .dbn files less enormous, e.g.
// go run ./broker/databento/cmd/dbnfilter -o ~/databento/SPXW/2026-03-20.cmbp-1.0dte.dbn -date 2026-03-20 -defs ~/databento/SPXW/2026-03-20.definition.dbn ~/databento/SPXW/2026-03-20.cmbp-1.dbn
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"

	"dropbear/broker/databento"
	"dropbear/clocky"
)

var (
	dateFlag   = clocky.TimeFlag("date", "", "date of the trades to report")
	outputFlag = flag.String("o", "", "output path")
	defsFlag   = flag.String("defs", "", "path to DBN file containing instrument definitions")
)

var (
	gInstrumentsByID = map[uint32]*databento.Instrument{}
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: dbn0dte -o out.dbn -date YYYY-MM-DD -defs definitions.dbn <file.dbn> ...\n")
		os.Exit(1)
	}
	if *outputFlag == "" {
		fmt.Fprintf(os.Stderr, "output path is required\n")
		os.Exit(1)
	}
	if *defsFlag == "" {
		fmt.Fprintf(os.Stderr, "definitions path is required\n")
		os.Exit(1)
	}

	// load definitions
	defReader, err := databento.OpenFileReader(*defsFlag)
	if err != nil {
		fmt.Printf("%s: %v\n", *defsFlag, err)
		os.Exit(1)
	}
	defer defReader.Close()
	if defReader.Metadata.Schema != databento.SchemaDefinition {
		fmt.Printf("%s: expected definitions DBN file with schema %d, got %d\n", *defsFlag, databento.SchemaDefinition, defReader.Metadata.Schema)
		os.Exit(1)
	}
	wantYear, wantMonth, wantDay := dateFlag.Date()
	for {
		rec, err := defReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("%s: read error: %v\n", *defsFlag, err)
			os.Exit(1)
		}
		switch m := rec.(type) {
		case *databento.Instrument:
			year, timeMonth, day := m.Expiration.In(clocky.UTC).Date()
			month := clocky.Month(timeMonth)
			if year == wantYear && month == wantMonth && day == wantDay {
				id := m.Header.InstrumentID
				gInstrumentsByID[id] = m
			}
		}
	}
	if len(gInstrumentsByID) == 0 {
		fmt.Fprintf(os.Stderr, "no definitions found\n")
		os.Exit(1)
	}

	// read and write data records
	outFile, err := os.Create(*outputFlag)
	if err != nil {
		fmt.Printf("%s: %v\n", *outputFlag, err)
		os.Exit(1)
	}
	defer outFile.Close()
	out := bufio.NewWriter(outFile)
	isFirst := true
	for _, path := range flag.Args() {
		dbnReader, err := databento.OpenFileReader(path)
		if err != nil {
			fmt.Printf("%s: %v\n", path, err)
			os.Exit(1)
		}
		defer dbnReader.Close()
		if isFirst {
			if _, err := out.Write(dbnReader.Metadata.Encode()); err != nil {
				fmt.Printf("%s: write metadata: %v\n", *outputFlag, err)
				os.Exit(1)
			}
			isFirst = false
		}
		for {
			rec, err := dbnReader.Read()
			if err != nil {
				if err == io.EOF {
					break
				}
				fmt.Printf("%s: read error: %v\n", path, err)
				os.Exit(1)
			}
			dbn := rec.(databento.DBN)
			_, ok := gInstrumentsByID[dbn.InstrumentID()]
			if !ok {
				continue
			}
			if _, err := out.Write(dbn.Encode()); err != nil {
				fmt.Printf("%s: write record: %v\n", *outputFlag, err)
				os.Exit(1)
			}
		}
	}
	if err := out.Flush(); err != nil {
		fmt.Printf("%s: flush: %v\n", *outputFlag, err)
		os.Exit(1)
	}
}
