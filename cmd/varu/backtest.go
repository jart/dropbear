package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/options"
	"dropbear/ds/symbol"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

func backtest() {
	if *dbnFlag == "" {
		fmt.Fprintln(os.Stderr, "missing required -dbn flag")
		os.Exit(1)
	}
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
	ready := false
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
			if clock >= kMarketClose {
				break
			}
			if clock < kStartOfDay {
				continue
			}
			if !ready && gChain.AtTheMoney != nil {
				onOptionDefEnd()
				ready = true
			}
			if ready && now >= nextThought && now >= gNextTradeTime {
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
	if *webFlag {
		broadcastState()
	}
	onEndOfDay()
}

func simulateOrder(sim *Simulation) {
	gVolume += sim.Legs.Size()
	gTotalFees = gTotalFees.Add(kFeePerContract.MulInt(sim.Legs.Size()))
	spread := *spreadFlag
	for legIt := sim.Legs.Iterator(); legIt.Next(); {
		sym, pos := legIt.Key(), legIt.Value()
		opt := gOptionsByOSI[sym]
		mid := opt.MarketPrice()
		hlf := opt.Ask.Sub(opt.Bid).DivInt(2)
		var fillPrice decimal.Decimal
		if pos.IsPositive() {
			fillPrice = quantizeTruncate(mid.Add(hlf.Mul(spread)))
		} else {
			fillPrice = quantizeAway(mid.Sub(hlf.Mul(spread)))
		}
		cashFlow := fillPrice.Mul(pos.Neg()).MulInt(kMultiplier)
		gCash = gCash.Add(cashFlow)
		if cashFlow.IsPositive() {
			gTotalCashIn = gTotalCashIn.Add(cashFlow)
		} else {
			gTotalCashOut = gTotalCashOut.Add(cashFlow.Neg())
		}
		existing, _ := gPositions.Get(sym)
		gPositions.Put(sym, existing.Add(pos))
		recordFill(sym, pos, fillPrice)
	}
	sanityCheck("simulateOrder")
}
