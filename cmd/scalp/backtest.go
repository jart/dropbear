package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/options"
	"dropbear/osi"
	"dropbear/symbol"
	"fmt"
	"io"
	"log"
	"slices"
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
			rs := m.GetRawSymbol()
			sym, strike, class, expYear, expMonth, expDay, err := osi.Parse(rs)
			if err == nil {
				if sym == t.Symbol && expYear == wantYear && expMonth == wantMonth && expDay == wantDay {
					t.onOptionDef(&options.Option{
						ID:     m.Header.InstrumentID,
						Class:  databento.InstrumentClass(class),
						Strike: &options.Strike{Price: strike},
						Symbol: sym,
						Year:   expYear,
						Month:  expMonth,
						Day:    expDay,
					})
				} else {
					log.Printf("skipping instrument %s expiring on %04d-%02d-%02d\n", sym, expYear, expMonth, expDay)
				}
			} else if sym, err = symbol.Parse(rs); err == nil {
				t.onEquityDef(&options.Equity{
					ID:     m.Header.InstrumentID,
					Symbol: sym,
				})
			} else {
				log.Printf("skipping instrument %s\n", rs)
			}
		case *databento.MBP1:
			e := t.onEquityTick(m)
			if e == nil {
				continue
			}
			t.simulateFills(e)
		case *databento.CMBP1:
			now := m.TSRecv
			clocky.SetNow(now)
			o := t.onOptionTick(m)
			if o == nil {
				continue
			}
			t.simulateFills(o)
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
			if now.After(nextHeartbeat) {
				nextHeartbeat = now.Add(*heartbeatFlag)
				t.onHeartbeat()
			}
		default:
			return fmt.Errorf("unexpected record type %T", rec)
		}
	}
	t.onEndOfDay()
	return nil
}

func (t *Trader) simulateOrder(order *Order) {
	t.addPendingOrder(order)
}

func (t *Trader) simulateFills(security options.Security) {
	for _, order := range slices.Clone(t.PendingOrdersBySecurity[security]) {
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
			tick, _ := getTicks(order.Legs[0].Security.GetSymbol())
			priceThatMarketDemands = priceThatMarketDemands.QuantizeFloor(tick)
		} else {
			tick, bigTick := getTicks(order.Legs[0].Security.GetSymbol())
			if priceThatMarketDemands.Abs().Cmp(kThree) >= 0 {
				tick = bigTick
			}
			priceThatMarketDemands = priceThatMarketDemands.QuantizeFloor(tick)
		}
	}

	// market orders should fill immediately at price that market demands
	if order.Price.IsZero() {
		order.Price = priceThatMarketDemands
	}

	// determine if this order is able to be filled
	// e.g. order.price is -.5 (debit/buy) would fill when market moves from -.6 to -.4
	// e.g. order.price is +.5 (credit/sell) would fill when market moves from +.4 to +.6
	if priceThatMarketDemands.Cmp(order.Price) < 0 {
		return false // market hasn't reached our limit yet
	}

	// check all legs have valid fill prices before committing
	for _, leg := range order.Legs {
		if leg.Quantity.IsPositive() && !leg.Security.GetAsk().IsPositive() {
			return false // can't buy at zero ask
		}
		if leg.Quantity.IsNegative() && !leg.Security.GetBid().IsPositive() {
			return false // can't sell at zero bid
		}
	}

	// simulate fill by updating holdings and cash
	log.Printf("simulated fill of order #%d at price %s -> %s\n",
		order.ID, order.Price, priceThatMarketDemands)
	for _, leg := range order.Legs {
		var fillPrice decimal.Decimal
		if *hostileFlag {
			if leg.Quantity.IsPositive() {
				fillPrice = leg.Security.GetAsk()
			} else {
				fillPrice = leg.Security.GetBid()
			}
		} else {
			fillPrice = leg.Security.MidPrice()
		}
		t.Holdings.Add(leg.Security, leg.Quantity, fillPrice)
		// fee := kFeePerContract.Mul(leg.Quantity.Abs())
		// t.Holdings.TotalFees = t.Holdings.TotalFees.Add(fee)
		leg.Filled = true
	}

	// caller should now call removePendingOrder if it was added
	return true
}

func (t *Trader) simulateCancelOrder(order *Order) {
	t.removePendingOrder(order)
}
