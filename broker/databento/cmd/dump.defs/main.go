// Command dump.defs fetches instrument definitions from Databento's historical
// API and prints every field. Useful for inspecting data to verify struct layout.
//
// Usage:
//
//	go run ./broker/databento/cmd/dump.defs -date 2026-02-27
//	go run ./broker/databento/cmd/dump.defs -date 2026-02-27 -dataset OPRA.PILLAR -symbol SPXW.OPT
package main

import (
	"flag"
	"fmt"
	"log"
	"time"
	"unsafe"

	"dropbear/broker/databento"
)

func main() {
	dateStr := flag.String("date", "", "query date in YYYY-MM-DD format (required)")
	dataset := flag.String("dataset", "OPRA.PILLAR", "Databento dataset")
	symbol := flag.String("symbol", "SPXW.OPT", "parent symbol to query")
	flag.Parse()

	if *dateStr == "" {
		log.Fatal("flag -date is required (e.g. -date 2026-02-27)")
	}

	date, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		log.Fatalf("bad date %q: %v", *dateStr, err)
	}
	start := date.Format("2006-01-02") + "T00:00:00Z"
	end := date.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00Z"

	// Query date as YYYYMMDD integer for filtering 0DTE
	queryDateInt := date.Year()*10000 + int(date.Month())*100 + date.Day()

	apiKey, err := databento.GetKey()
	if err != nil {
		log.Fatalf("read API key: %v", err)
	}

	client := databento.NewHistoricalClient(apiKey)
	fmt.Printf("fetching definitions: dataset=%s symbol=%s start=%s end=%s\n", *dataset, *symbol, start, end)

	meta, records, err := client.GetRange(*dataset, databento.SchemaDefinition,
		databento.STypeParent, []string{*symbol}, start, end)
	if err != nil {
		log.Fatalf("GetRange: %v", err)
	}

	fmt.Println(databento.Indent(&meta))
	fmt.Printf("total records: %d\n", len(records))

	// Filter for 0DTE instruments and print all fields
	printed := 0
	for i, rec := range records {
		if len(rec) < int(unsafe.Sizeof(databento.Instrument{})) {
			fmt.Printf("\nrecord %d: too small (%d bytes, need %d)\n",
				i, len(rec), unsafe.Sizeof(databento.Instrument{}))
			continue
		}
		inst := (*databento.Instrument)(unsafe.Pointer(&rec[0]))

		// Filter: only 0DTE (expiration UTC date matches query date)
		expUTC := time.Unix(int64(inst.Expiration)/1e9, int64(inst.Expiration)%1e9).UTC()
		expDateInt := expUTC.Year()*10000 + int(expUTC.Month())*100 + expUTC.Day()
		if expDateInt != queryDateInt {
			continue
		}

		printed++
		fmt.Println(databento.Indent(inst))
	}
	fmt.Printf("\n0DTE instruments: %d (of %d total)\n", printed, len(records))
}
