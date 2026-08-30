package main

import (
	"dropbear/broker/databento"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/netty"
	"dropbear/options"
	"dropbear/osi"
	"dropbear/symbol"
	"fmt"
	"io"
	"log"
)

// streamEquities subscribes to equity definitions and quotes on a single live
// gateway connection.
func (t *Trader) streamEquities(key databento.ApiKey, defs chan<- *options.Equity, ticks chan<- *databento.MBP1) {
	netty.Reconnect("equity stream", func() error {
		return t.streamEquitiesOnce(key, defs, ticks)
	}, log.Printf)
}

func (t *Trader) streamEquitiesOnce(key databento.ApiKey, defs chan<- *options.Equity, ticks chan<- *databento.MBP1) error {
	client, err := databento.Dial("EQUS.MINI", key)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer client.Close()
	dbSymbol := fmt.Sprintf("%s", t.Config.Symbol)
	if err := client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeRawSymbol,
		Symbols: []string{dbSymbol},
	}); err != nil {
		return fmt.Errorf("subscribe definitions: %w", err)
	}
	if err := client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaMBP1,
		SType:   databento.STypeRawSymbol,
		Symbols: []string{dbSymbol},
	}); err != nil {
		return fmt.Errorf("subscribe quotes: %w", err)
	}
	meta, err := client.Start()
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	log.Printf("streaming %s (dbn v%d)", dbSymbol, meta.Version)
	for {
		rec, err := client.Read()
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("stream closed")
			}
			return fmt.Errorf("decode: %w", err)
		}
		switch m := rec.(type) {
		case *databento.ErrorMsg:
			return fmt.Errorf("equity gateway error: %s", m.Err)
		case *databento.SystemMsg:
			log.Printf("equity system message: %s", m.Msg)
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
	netty.Reconnect("option stream", func() error {
		return t.streamOptionsOnce(key, defs, ticks)
	}, log.Printf)
}

func (t *Trader) streamOptionsOnce(key databento.ApiKey, defs chan<- *options.Option, ticks chan<- *databento.CMBP1) error {
	client, err := databento.Dial("OPRA.PILLAR", key)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer client.Close()
	dbSymbol := fmt.Sprintf("%s.OPT", t.Config.Symbol)
	if err := client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeParent,
		Symbols: []string{dbSymbol},
	}); err != nil {
		return fmt.Errorf("subscribe definitions: %w", err)
	}
	if err := client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaCMBP1,
		SType:   databento.STypeParent,
		Symbols: []string{dbSymbol},
	}); err != nil {
		return fmt.Errorf("subscribe quotes: %w", err)
	}
	meta, err := client.Start()
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	log.Printf("streaming %s (dbn v%d)", dbSymbol, meta.Version)
	wantYear, wantMonth, wantDay := clocky.Now().Date()
	wantYear, wantMonth, wantDay = cboe.GetNextOptionChain(t.Config.Symbol, wantYear, wantMonth, wantDay)
	log.Printf("%s shall use %04d-%02d-%02d chain", t.Config.Symbol, wantYear, wantMonth, wantDay)
	for {
		rec, err := client.Read()
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("stream closed")
			}
			return fmt.Errorf("decode: %w", err)
		}
		switch m := rec.(type) {
		case *databento.ErrorMsg:
			return fmt.Errorf("option gateway error: %s", m.Err)
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
