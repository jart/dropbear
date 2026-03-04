package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/ds/symbol"
	"io"
	"log"
	"sync"
)

var (
	futuresLock sync.RWMutex
	futuresByID = make(map[uint32]*Future)
	es          *Future
	sr1         *Future
)

func monitorFuturesLive(key databento.ApiKey) {
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
				break
			}
			log.Fatalf("decode: %v", err)
		}
		switch m := rec.(type) {
		case *databento.ErrorMsg:
			log.Fatalf("future gateway error: %s", m.Err)
		case *databento.SystemMsg:
			log.Printf("future system message: %s", m.Msg)
		case *databento.SymbolMappingMsg:
			onFutureMap(key, m.Header.InstrumentID, m.GetSTypeOutSymbol())
		case *databento.MBP1:
			onFutureTick(m)
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
}

func onFutureMap(key databento.ApiKey, id uint32, str string) {
	log.Printf("map %d -> %s", id, str)
	sym, year, month, err := parseCME(str)
	if err != nil {
		log.Printf("failed to parse cme symbol: %v", err)
		return
	}
	future := &Future{
		ID:    id,
		Sym:   sym,
		Year:  year,
		Month: month,
	}
	futuresByID[id] = future
	futuresLock.Lock()
	defer futuresLock.Unlock()
	if sym == symbol.Symbol('E'|'S'<<8) {
		es = future
	} else if sym == symbol.Symbol('S'|'R'<<8|'1'<<16) {
		sr1 = future
		// sr1 updates very infrequently, so initialize price from historical api
		go fetchFuturePrice(key, sr1)
	} else {
		log.Fatalf("unknown future symbol: %s", sym)
	}
}

func onFutureTick(m *databento.MBP1) {
	futuresLock.Lock()
	defer futuresLock.Unlock()
	future := futuresByID[m.Header.InstrumentID]
	if future == nil {
		return
	}
	updateFuturePrice(m, future)
}

func updateFuturePrice(m *databento.MBP1, future *Future) {
	bid := dbnPrice(m.Levels[0].BidPx)
	ask := dbnPrice(m.Levels[0].AskPx)
	mid := bid.Add(ask).DivInt(2)
	future.Bid = bid
	future.Ask = ask
	future.Price = mid
	log.Printf("tick %s mid %s bid %s ask %s", future.Sym, mid, bid, ask)
}

func fetchFuturePrice(key databento.ApiKey, future *Future) {
	client := databento.NewHistoricalClient(key)
	start := clocky.Now().Add(-clocky.Day).In(clocky.UTC)
	end := clocky.Now().In(clocky.UTC)
	startStr := start.Format("2006-01-02")
	endStr := end.Format("2006-01-02")
	symbols := []string{future.CME()}
	_, recs, err := client.GetRange("GLBX.MDP3", databento.SchemaMBP1, databento.STypeRawSymbol, symbols, startStr, endStr, 1)
	if err != nil {
		log.Fatalf("failed to fetch historical mbp1: %v", err)
	}
	if len(recs) == 0 {
		log.Fatalf("no historical mbp1 records found for %s", future)
	}
	rec := recs[0].(*databento.MBP1)
	futuresLock.Lock()
	defer futuresLock.Unlock()
	updateFuturePrice(rec, future)
}
