package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/ds/cme"
	"io"
	"log"
)

func streamFutures(key databento.ApiKey, defs chan<- *Future, ticks chan<- *databento.MBP1) {
	client, err := databento.Dial("GLBX.MDP3", key)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()
	symbols := []string{"ES.c.0", "SR1.c.0"}
	client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeContinuous,
		Symbols: symbols,
	})
	client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaMBP1,
		SType:   databento.STypeContinuous,
		Symbols: symbols,
	})
	_, err = client.Start()
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	for {
		rec, err := client.Read()
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Fatalf("decode: %v", err)
		}
		switch m := rec.(type) {
		case *databento.ErrorMsg:
			log.Fatalf("future gateway error: %s", m.Err)
		case *databento.SystemMsg:
			log.Printf("future system message: %s", m.Msg)
		case *databento.SymbolMappingMsg:
			id := m.Header.InstrumentID
			str := m.GetSTypeOutSymbol()
			log.Printf("map %d -> %s", id, str)
			sym, year, month, err := cme.Parse(str)
			if err != nil {
				log.Printf("failed to parse cme symbol: %v", err)
				continue
			}
			defs <- &Future{
				ID:     id,
				Symbol: sym,
				Year:   year,
				Month:  month,
			}
		case *databento.MBP1:
			ticks <- m
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
}

func fetchFuturePrice(key databento.ApiKey, future *Future, ticks chan<- *databento.MBP1) {
	client := databento.NewHistoricalClient(key)
	start := clocky.Now().Add(-clocky.Day)
	end := clocky.Now()
	symbols := []string{future.CME()}
	// "GLBX.MDP3", databento.SchemaMBP1, databento.STypeRawSymbol, symbols, startStr, endStr, 1)
	response, err := client.GetRange(databento.GetRangeParams{
		Dataset:  "GLBX.MDP3",
		Schema:   databento.SchemaMBP1,
		STypeIn:  databento.STypeRawSymbol,
		STypeOut: databento.STypeContinuous,
		Symbols:  symbols,
		Start:    start,
		End:      end,
		Limit:    1,
	})
	if err != nil {
		log.Fatalf("failed to fetch historical mbp1: %v", err)
	}
	defer response.Close()
	record, err := response.Read()
	if err != nil {
		log.Fatalf("failed to read historical mbp1: %v", err)
	}
	ticks <- record.(*databento.MBP1)
}
