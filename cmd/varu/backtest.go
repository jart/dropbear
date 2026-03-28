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
	"slices"
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
			symbol := symbol.MustParse(m.GetAsset())
			year, timeMonth, day := m.Expiration.In(clocky.UTC).Date()
			month := clocky.Month(timeMonth)
			if symbol == gSymbol && year == wantYear && month == wantMonth && day == wantDay {
				id := m.Header.InstrumentID
				strike := decimal.Decimal(m.StrikePrice / 1000)
				class := m.InstrumentClass
				option := &options.Option{
					ID:     id,
					Class:  class,
					Strike: &options.Strike{Price: strike},
					Symbol: symbol,
					Year:   year,
					Month:  month,
					Day:    day,
				}
				onOptionDef(option)
			} else {
				log.Printf("skipping instrument %s expiring on %04d-%02d-%02d\n", symbol, year, month, day)
			}
		case *databento.CMBP1:
			now := m.TSRecv
			clocky.SetNow(now)
			o := onOptionTick(m)
			simulateFills(o)
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
			if ready && now >= nextThought {
				nextThought = now.Add(*thinkFlag)
				onData(now)
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

func simulateOrder(order *Order) {
	// order will be filled immediately if it crosses the spread
	if !simulateFillOrder(order) {
		// otherwise we wait for market to move towards limit price
		addPendingOrder(order)
	}
}

func simulateFills(option *options.Option) {
	for _, order := range slices.Clone(gPendingOrdersByOption[option]) {
		if simulateFillOrder(order) {
			removePendingOrder(order)
		}
	}
}

func simulateFillOrder(order *Order) bool {
	if order.Filled() {
		panic("order already filled")
	}

	// determine if this order is able to be filled
	// we assume maximally hostile market that requires crossing the spread
	// if you order at the mid, you'll have to wait for market to move toward you
	// e.g. order.price is -.5 (debit/buy) would fill when market moves from -.6 to -.4
	// e.g. order.price is +.5 (credit/sell) would fill when market moves from +.4 to +.6
	priceThatMarketDemands := order.NaturalPrice()
	if priceThatMarketDemands.Cmp(order.Price) < 0 {
		return false // market hasn't reached our limit yet
	}

	// simulate fill by updating holdings and cash
	log.Printf("simulated fill of order #%d at price %s -> %s\n",
		order.ID, order.Price, priceThatMarketDemands)
	for _, leg := range order.Legs {
		var fillPrice decimal.Decimal
		if leg.Quantity.IsPositive() {
			fillPrice = leg.Option.Ask // buy high :'(
		} else {
			fillPrice = leg.Option.Bid // sell low :'(
		}
		gHoldings.Add(leg.Option, leg.Quantity, fillPrice)
		fee := kFeePerContract.Mul(leg.Quantity.Abs())
		gHoldings.TotalFees = gHoldings.TotalFees.Add(fee)
		leg.Filled = true
	}

	// caller should now call removePendingOrder if it was added
	return true
}

func simulateCancelOrder(order *Order) {
	removePendingOrder(order)
}
