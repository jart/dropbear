package main

import (
	"flag"
	"fmt"
	"log"

	"dropbear/broker/databento"
)

func main() {
	symbol := flag.String("symbol", "SPY", "symbol to probe")
	dataset := flag.String("dataset", "OPRA.PILLAR", "databento dataset")
	flag.Parse()

	apiKey := databento.MustLoadDefaultKey()
	client, err := databento.Dial(*dataset, apiKey)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()

	parentSym := *symbol + ".OPT"
	log.Printf("Subscribing to %s...", parentSym)

	// Subscribe to definitions (replaying from today's open to get full table)
	err = client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeParent,
		Symbols: []string{parentSym},
	})
	if err != nil {
		log.Fatalf("subscribe: %v", err)
	}

	meta, err := client.Start()
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	log.Printf("Session started. DBN version: %d", meta.Version)

	expirations := make(map[string]int)
	for i := 0; i < 50000; i++ {
		rec, err := client.NextRecord()
		if err != nil {
			log.Fatalf("next record: %v", err)
		}

		rtype := databento.RType(rec[1])
		if rtype == databento.RTypeSymbolMapping {
			osi := databento.CastRecord(rec)
			if m, ok := osi.(*databento.SymbolMappingMsg); ok {
				out := m.GetSTypeOutSymbol()
				if len(out) >= 12 {
					expirations[out[6:12]]++
					if out[6:12] == "260302" {
						fmt.Printf("FOUND 0DTE: in=%q out=%q id=%d\n", 
							m.GetSTypeInSymbol(), out, m.Header.InstrumentID)
					}
				}
			}
		}
		if i%10000 == 0 {
			log.Printf("Processed %d records...", i)
		}
	}
	fmt.Println("\nUnique Expirations Found:")
	for exp, count := range expirations {
		fmt.Printf("  %s: %d mappings\n", exp, count)
	}
}
