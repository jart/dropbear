package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/options"
	"dropbear/ds/symbol"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

var (
	defsFlag   = flag.String("defs", "", "path to DBN file containing SPX definitions")
	quotesFlag = flag.String("quotes", "", "path to DBN file containing SPX quotes")
	thinkFlag  = clocky.DurationFlag("think", "250ms", "interval between trading analysis")
)

func backtest() {
	loadDefinitions(*defsFlag)
	quoteReader, err := databento.OpenFileReader(*quotesFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", *quotesFlag, err)
		os.Exit(1)
	}
	defer quoteReader.Close()
	if quoteReader.Metadata.Schema != databento.SchemaCMBP1 {
		fmt.Fprintf(os.Stderr, "%s: expected quotes DBN file with schema %d, got %d\n",
			*quotesFlag, databento.SchemaCMBP1, quoteReader.Metadata.Schema)
		os.Exit(1)
	}
	clocky.Now = clocky.FakeNow
	clocky.Sleep = clocky.FakeSleep
	var rtBase time.Time
	var rtBaseData clocky.Time
	var nextThought, nextHeartbeat clocky.Time
	for {
		rec, err := quoteReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "%s: read quote error: %v\n", *quotesFlag, err)
			os.Exit(1)
		}
		m := rec.(*databento.CMBP1)
		now := m.TSRecv
		clocky.SetNow(now)
		onOptionTick(m)
		clock := now.ClockInt()
		if *rtFlag {
			if rtBaseData == 0 && clock >= kStartOfDay {
				rtBase = time.Now()
				rtBaseData = now
			}
			if rtBaseData != 0 {
				dataElapsed := time.Duration(now - rtBaseData)
				wallElapsed := time.Since(rtBase)
				if dataElapsed > wallElapsed {
					time.Sleep(dataElapsed - wallElapsed)
				}
			}
		}
		if clock >= kEndOfDay {
			break
		}
		if clock < kStartOfDay {
			continue
		}
		if now >= nextThought && now >= gNextTradeTime && clock <= kStopTrading {
			nextThought = now.Add(*thinkFlag)
			onThink(now)
		}
		if *webFlag {
			for {
				select {
				case req := <-gWebRequests:
					processWebRequest(req)
				default:
					goto doneWebRequests
				}
			}
		doneWebRequests:
		}
		if now >= nextHeartbeat {
			nextHeartbeat = now.Add(*heartbeatFlag)
			onHeartbeat()
			if *webFlag {
				broadcastState()
			}
		}
	}
	onEndOfDay()
}

func loadDefinitions(path string) {
	defReader, err := databento.OpenFileReader(path)
	if err != nil {
		fmt.Printf("%s: %v\n", path, err)
		os.Exit(1)
	}
	defer defReader.Close()
	if defReader.Metadata.Schema != databento.SchemaDefinition {
		fmt.Printf("%s: expected definitions DBN file with schema %d, got %d\n", path, databento.SchemaDefinition, defReader.Metadata.Schema)
		os.Exit(1)
	}
	wantYear, wantMonth, wantDay := dateFlag.Date()
	for {
		rec, err := defReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("%s: read error: %v\n", path, err)
			os.Exit(1)
		}
		switch m := rec.(type) {
		case *databento.Instrument:
			sym := symbol.MustParse(m.GetAsset())
			year, timeMonth, day := m.Expiration.In(clocky.UTC).Date()
			month := clocky.Month(timeMonth)
			if sym == gSymbol && year == wantYear && month == wantMonth && day == wantDay {
				id := m.Header.InstrumentID
				strike := decimal.Decimal(m.StrikePrice / 1000)
				class := m.InstrumentClass
				option := &options.Option{
					ID:     id,
					Class:  class,
					Strike: &options.Strike{Price: strike},
					Sym:    sym,
					Year:   year,
					Month:  month,
					Day:    day,
				}
				onOptionDef(option)
			}
		}
	}
	if len(gOptionsByID) == 0 {
		fmt.Fprintf(os.Stderr, "no %s 0dte definitions found\n", gSymbol)
		os.Exit(1)
	}
}

func simulateOrder(sim *Simulation) {
	gCash = gCash.Add(sim.Price.MulInt(kMultiplier))
	for legIt := sim.Legs.Iterator(); legIt.Next(); {
		sym, pos := legIt.Key(), legIt.Value()
		existing, _ := gPositions.Get(sym)
		gPositions.Put(sym, existing.Add(pos))
	}
}
