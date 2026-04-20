package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/options"
	"dropbear/osi"
	"dropbear/symbol"
	"fmt"
	"io"
	"log"
)

func (t *Trader) streamEquities(key databento.ApiKey, defs chan<- *options.Equity, ticks chan<- *databento.MBP1) {
	client, err := databento.Dial("EQUS.MINI", key)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()
	dbSymbol := fmt.Sprintf("%s", t.Config.Symbol)
	client.MustSubscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeRawSymbol,
		Symbols: []string{dbSymbol},
	})
	client.MustSubscribe(databento.Subscription{
		Schema:  databento.SchemaMBP1,
		SType:   databento.STypeRawSymbol,
		Symbols: []string{dbSymbol},
	})
	meta := client.MustStart()
	log.Printf("streaming %s (dbn v%d)", dbSymbol, meta.Version)
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
			sym, err := symbol.Parse(str)
			if err != nil {
				continue
			}
			if sym != t.Config.Symbol {
				continue
			}
			log.Printf("got equity definition: %s (id %d)", str, id)
			defs <- &options.Equity{
				ID:     id,
				Symbol: sym,
			}
		case *databento.MBP1:
			ticks <- m
		case *databento.Instrument:
			// ignore raw definitions; we use SymbolMappingMsg instead
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
}

// streamOptions subscribes to both definitions and quotes on a single
// live gateway connection. Definitions arrive via SymbolMappingMsg which
// maps parent symbols to instrument IDs with OSI names. This avoids the
// historical API which can time out.
func (t *Trader) streamOptions(key databento.ApiKey, defs chan<- *options.Option, ticks chan<- *databento.CMBP1) {
	client, err := databento.Dial("OPRA.PILLAR", key)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()
	dbSymbol := fmt.Sprintf("%s.OPT", t.Config.Symbol)
	client.MustSubscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeParent,
		Symbols: []string{dbSymbol},
	})
	client.MustSubscribe(databento.Subscription{
		Schema:  databento.SchemaCMBP1,
		SType:   databento.STypeParent,
		Symbols: []string{dbSymbol},
	})
	meta := client.MustStart()
	log.Printf("streaming %s (dbn v%d)", dbSymbol, meta.Version)
	wantYear, wantMonth, wantDay := clocky.Now().Date()
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
			sym, strike, class, year, monthy, day, err := osi.Parse(str)
			if err != nil {
				continue
			}
			if sym != t.Config.Symbol || year != wantYear || clocky.Month(monthy) != wantMonth || day != wantDay {
				continue
			}
			log.Printf("got option definition: %s (id %d)", str, id)
			defs <- &options.Option{
				ID:     id,
				Class:  databento.InstrumentClass(class),
				Strike: &options.Strike{Price: strike},
				Symbol: sym,
				Year:   year,
				Month:  clocky.Month(monthy),
				Day:    day,
			}
		case *databento.CMBP1:
			ticks <- m
		case *databento.Instrument:
			// ignore raw definitions; we use SymbolMappingMsg instead
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
}
