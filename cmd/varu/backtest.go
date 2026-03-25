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
	"log"
	"os"
	"time"
)

var (
	dbnFlag   = flag.String("dbn", "", "path to DBN file containing SPX definitions and quotes")
	thinkFlag = clocky.DurationFlag("think", "250ms", "interval between trading analysis")
)

func backtest() {
	quoteReader, err := databento.OpenFileReader(*dbnFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", *dbnFlag, err)
		os.Exit(1)
	}
	defer quoteReader.Close()
	if quoteReader.Metadata.Schema != databento.SchemaCMBP1 {
		fmt.Fprintf(os.Stderr, "%s: expected quotes DBN file with schema %d, got %d\n",
			*dbnFlag, databento.SchemaCMBP1, quoteReader.Metadata.Schema)
		os.Exit(1)
	}
	clocky.Now = clocky.FakeNow
	clocky.Sleep = clocky.FakeSleep
	loadedOptionDefs := false
	var rtBase time.Time
	var nextDump time.Time
	var rtBaseData clocky.Time
	var nextThought, nextHeartbeat clocky.Time
	wantYear, wantMonth, wantDay := dateFlag.Date()
	for {
		rec, err := quoteReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "%s: read quote error: %v\n", *dbnFlag, err)
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
			} else {
				log.Printf("skipping instrument %s expiring on %04d-%02d-%02d\n", sym, year, month, day)
			}
		case *databento.CMBP1:
			if !loadedOptionDefs {
				if len(gOptionsByID) == 0 {
					fmt.Fprintf(os.Stderr, "no %s 0dte definitions found\n", gSymbol)
					os.Exit(1)
				}
				onOptionDefEnd()
				loadedOptionDefs = true
			}
			now := m.TSRecv
			clocky.SetNow(now)
			onOptionTick(m)
			clock := now.ClockInt()
			var realNow time.Time
			if *rtFlag || *webFlag {
				realNow = time.Now()
			}
			if *rtFlag {
				if rtBaseData == 0 && clock >= kStartOfDay {
					rtBase = realNow
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
			if now >= nextThought && now >= gNextTradeTime {
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
				if realNow.After(nextDump) {
					nextDump = realNow.Add(slowdownFlag.Go())
					broadcastState()
				}
			}
			if now.After(nextHeartbeat) {
				nextHeartbeat = now.Add(*heartbeatFlag)
				onHeartbeat()
			}
		default:
			fmt.Fprintf(os.Stderr, "%s: unexpected record type %T\n", *dbnFlag, rec)
			os.Exit(1)
		}
	}
	onEndOfDay()
}

func simulateOrder(sim *Simulation) {
	gCash = gCash.Add(sim.Price.MulInt(kMultiplier))
	for legIt := sim.Legs.Iterator(); legIt.Next(); {
		sym, pos := legIt.Key(), legIt.Value()
		existing, _ := gPositions.Get(sym)
		gPositions.Put(sym, existing.Add(pos))
		recordFill(sym, pos, gOptionsByOSI[sym].MarketPrice())
	}
}
