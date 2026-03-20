package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/ds/osi"
	"io"
	"log"
	"time"
)

func streamOptions(key databento.ApiKey, defs chan<- *Option, ticks chan<- *databento.CMBP1) {
	client, err := databento.Dial("OPRA.PILLAR", key)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()
	client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeParent,
		Symbols: []string{"SPXW.OPT"},
	})
	client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaCMBP1,
		SType:   databento.STypeParent,
		Symbols: []string{"SPXW.OPT"},
	})
	meta, err := client.Start()
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	log.Printf("streaming %s (dbn v%d)", "SPXW.OPT", meta.Version)
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
			log.Fatalf("option gateway error: %s", m.Err)
		case *databento.SystemMsg:
			log.Printf("option system message: %s", m.Msg)
		case *databento.SymbolMappingMsg:
			id := m.Header.InstrumentID
			str := m.GetSTypeOutSymbol()
			sym, strike, class, year, month, day, err := osi.Parse(str)
			if err != nil {
				log.Printf("failed to parse OSI: %v", err)
				continue
			}
			now := clocky.Now().In(clocky.UTC)
			todayYear, todayMonth, todayDay := now.Date()
			if year != todayYear || time.Month(month) != todayMonth || day != todayDay {
				continue
			}
			defs <- &Option{
				ID:     id,
				Class:  databento.InstrumentClass(class),
				Sym:    sym,
				Strike: strike,
				Year:   year,
				Month:  month,
				Day:    day,
			}
		case *databento.CMBP1:
			ticks <- m
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
}
