package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"io"
	"log"
	"time"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

var (
	optionsByID     = make(map[uint32]*Option)
	optionsByStrike = treeset.NewWith(compareOptionByStrike)
)

func tradeOptionsLive(key databento.ApiKey) {
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
		Schema:  databento.SchemaTCBBO,
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
				break
			}
			log.Fatalf("decode: %v", err)
		}
		switch m := rec.(type) {
		case *databento.ErrorMsg:
			log.Fatalf("option gateway error: %s", m.Err)
		case *databento.SystemMsg:
			log.Printf("option system message: %s", m.Msg)
		case *databento.SymbolMappingMsg:
			onOptionMap(m.Header.InstrumentID, m.GetSTypeOutSymbol())
		case *databento.CBBO:
			onOptionTick(m.TSRecv, m.Header.InstrumentID, dbnPrice(m.Levels[0].BidPx), dbnPrice(m.Levels[0].AskPx))
		case *databento.CMBP1:
			onOptionTick(m.TSRecv, m.Header.InstrumentID, dbnPrice(m.Levels[0].BidPx), dbnPrice(m.Levels[0].AskPx))
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
}

func onOptionMap(id uint32, str string) {
	log.Printf("map %d -> %s", id, str)
	sym, strike, class, year, month, day, err := parseOSI(str)
	if err != nil {
		log.Printf("failed to parse OSI: %v", err)
		return
	}
	now := clocky.Now().In(clocky.UTC)
	todayYear, todayMonth, todayDay := now.Date()
	if year != todayYear || time.Month(month) != todayMonth || day != todayDay {
		return
	}
	option := &Option{
		ID:     id,
		Class:  class,
		Sym:    sym,
		Strike: strike,
		Year:   year,
		Month:  month,
		Day:    day,
	}
	optionsByID[id] = option
	optionsByStrike.Add(option)
}

func onOptionTick(ts clocky.Time, instID uint32, bid, ask decimal.Decimal) {
	ops := optionsByID[instID]
	if ops == nil {
		return
	}
	ops.Bid = bid
	ops.Ask = ask
	log.Printf("tick %s bid %s ask %s", ops, bid, ask)
}
