package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/options"
	"dropbear/symbol"
	"fmt"
	"io"
	"log"
	"slices"
	"time"
)

func (t *Trader) Backtest(dbn string, date clocky.Time) error {
	wantYear, wantMonth, wantDay := date.Date()
	log.Printf("starting backtest for %s on %04d-%02d-%02d",
		t.Symbol, wantYear, wantMonth, wantDay)
	quoteReader, err := databento.OpenFileReader(dbn)
	if err != nil {
		return err
	}
	defer quoteReader.Close()
	clocky.Now = clocky.FakeNow
	clocky.Sleep = clocky.FakeSleep
	ready := false
	var rtBase time.Time
	var nextDump time.Time
	var rtBaseData clocky.Time
	var nextThought, nextHeartbeat clocky.Time
	for {
		rec, err := quoteReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		switch m := rec.(type) {
		case *databento.Instrument:
			symbol := symbol.MustParse(m.GetAsset())
			year, timeMonth, day := m.Expiration.In(clocky.UTC).Date()
			month := clocky.Month(timeMonth)
			if symbol == t.Symbol && year == wantYear && month == wantMonth && day == wantDay {
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
				t.onOptionDef(option)
			} else {
				log.Printf("skipping instrument %s expiring on %04d-%02d-%02d\n", symbol, year, month, day)
			}
		case *databento.CMBP1:
			now := m.TSRecv
			clocky.SetNow(now)
			o := t.onOptionTick(m)
			if o == nil {
				continue
			}
			t.simulateFills(o)
			if _, ok := t.PendingOrdersByOption[o]; ok {
				t.cancelUnfilledOrders(now)
			}
			clock := now.ClockInt()
			var realNow time.Time
			if *rtFlag || t.Web != nil {
				realNow = time.Now()
			}
			if *rtFlag {
				if rtBaseData == 0 && clock >= t.Config.StartOfDay {
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
			if t.MarketClose != 0 && now.After(t.MarketClose) {
				break
			}
			if !ready && t.Chain.AtTheMoney != nil {
				t.onOptionDefEnd()
				ready = true
			}
			if ready && now >= nextThought {
				nextThought = now.Add(t.Config.Think)
				t.onThought(now)
			}
			if t.Web != nil {
				for {
					select {
					case req := <-t.Web.WebRequests:
						t.Web.processWebRequest(req)
					default:
						goto doneWebRequests
					}
				}
			doneWebRequests:
				if realNow.After(nextDump) {
					nextDump = realNow.Add(time.Duration(*slowdownFlag))
					t.Web.broadcastState()
				}
			}
			if now.After(nextHeartbeat) {
				nextHeartbeat = now.Add(*heartbeatFlag)
				t.onHeartbeat()
			}
		default:
			return fmt.Errorf("unexpected record type %T", rec)
		}
	}
	if t.Web != nil {
		t.Web.broadcastState()
	}
	t.onEndOfDay()
	return nil
}

func (t *Trader) simulateOrder(order *Order) {
	// order will be filled immediately if it crosses the spread
	if !t.simulateFillOrder(order) {
		// otherwise we wait for market to move towards limit price
		t.addPendingOrder(order)
	}
}

func (t *Trader) simulateFills(option *options.Option) {
	for _, order := range slices.Clone(t.PendingOrdersByOption[option]) {
		if t.simulateFillOrder(order) {
			t.removePendingOrder(order)
		}
	}
}

func (t *Trader) simulateFillOrder(order *Order) bool {
	if order.Filled() {
		panic("order already filled")
	}

	// choose execution price
	var priceThatMarketDemands decimal.Decimal
	if *hostileFlag {
		priceThatMarketDemands = order.NaturalPrice()
	} else {
		priceThatMarketDemands = order.MidPrice()
		if len(order.Legs) > 1 {
			tick, _ := getTicks(order.Legs[0].Option.Symbol)
			priceThatMarketDemands = priceThatMarketDemands.QuantizeFloor(tick)
		} else {
			tick, bigTick := getTicks(order.Legs[0].Option.Symbol)
			if priceThatMarketDemands.Abs().Cmp(kThree) >= 0 {
				tick = bigTick
			}
			priceThatMarketDemands = priceThatMarketDemands.QuantizeFloor(tick)
		}
	}

	// determine if this order is able to be filled
	// e.g. order.price is -.5 (debit/buy) would fill when market moves from -.6 to -.4
	// e.g. order.price is +.5 (credit/sell) would fill when market moves from +.4 to +.6
	if priceThatMarketDemands.Cmp(order.Price) < 0 {
		return false // market hasn't reached our limit yet
	}

	// check all legs have valid fill prices before committing
	for _, leg := range order.Legs {
		if leg.Quantity.IsPositive() && !leg.Option.Ask.IsPositive() {
			return false // can't buy at zero ask
		}
		if leg.Quantity.IsNegative() && !leg.Option.Bid.IsPositive() {
			return false // can't sell at zero bid
		}
	}

	// simulate fill by updating holdings and cash
	log.Printf("simulated fill of order #%d at price %s -> %s\n",
		order.ID, order.Price, priceThatMarketDemands)
	t.StrategiesUsed[order.Strategy] += 1
	for _, leg := range order.Legs {
		var fillPrice decimal.Decimal
		if *hostileFlag {
			if leg.Quantity.IsPositive() {
				fillPrice = leg.Option.Ask
			} else {
				fillPrice = leg.Option.Bid
			}
		} else {
			fillPrice = leg.Option.MidPrice()
		}
		t.Holdings.Add(leg.Option, leg.Quantity, fillPrice)
		fee := kFeePerContract.Mul(leg.Quantity.Abs())
		t.Holdings.TotalFees = t.Holdings.TotalFees.Add(fee)
		leg.Filled = true
	}

	// caller should now call removePendingOrder if it was added
	return true
}

func (t *Trader) simulateCancelOrder(order *Order) {
	t.removePendingOrder(order)
}
