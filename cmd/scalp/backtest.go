package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
	"dropbear/options"
	"dropbear/osi"
	"dropbear/symbol"
	"fmt"
	"io"
	"log"
	"slices"
)

func (t *Trader) Backtest(dbn string, date clocky.Time) error {
	netty.SetOffline()
	wantYear, wantMonth, wantDay := date.Date()
	log.Printf("starting backtest for %s on %04d-%02d-%02d",
		t.Config.Symbol, wantYear, wantMonth, wantDay)
	quoteReader, err := databento.OpenFileReader(dbn)
	if err != nil {
		return err
	}
	defer quoteReader.Close()
	clocky.Now = clocky.FakeNow
	clocky.Sleep = clocky.FakeSleep
	ready := false
	finished := false
	var nextHeartbeat clocky.Time
	for !finished {
		rec, err := quoteReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		var now clocky.Time
		switch m := rec.(type) {
		case *databento.Instrument:
			rs := m.GetRawSymbol()
			sym, strike, class, expYear, expMonth, expDay, err := osi.Parse(rs)
			if err == nil {
				if sym == t.Config.Symbol && expYear == wantYear && expMonth == wantMonth && expDay == wantDay {
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
			continue
		case *databento.MBP1:
			now = m.TSRecv
			clocky.SetNow(now)
			e := t.onEquityTick(m)
			if e == nil {
				continue
			}
			t.simulateFills(e)
		case *databento.CMBP1:
			now = m.TSRecv
			clocky.SetNow(now)
			o := t.onOptionTick(m)
			if o == nil {
				continue
			}
			t.simulateFills(o)
		default:
			return fmt.Errorf("unexpected record type %T", rec)
		}
		if t.MarketClose != 0 && now.After(t.MarketClose) {
			finished = true
		}
		if !ready && t.Chain.AtTheMoney != nil {
			t.onDefEnd()
			ready = true
		}
		if ready {
			t.onThought(now)
			if now.After(nextHeartbeat) {
				nextHeartbeat = now.Add(*heartbeatFlag)
				t.onHeartbeat()
			}
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

	// simulate network and broker latency
	now := clocky.Now()
	if now.Before(order.Created.Add(*latencyFlag)) {
		return false
	}

	// choose execution price
	var priceThatMarketDemands decimal.Decimal
	if *hostileFlag {
		priceThatMarketDemands = order.NaturalPrice()
	} else {
		priceThatMarketDemands = order.MidPrice()
	}
	var roundedPrice decimal.Decimal
	tick, bigTick := order.Ticks()
	if priceThatMarketDemands.Abs().Cmp(decimal.Three) >= 0 {
		roundedPrice = priceThatMarketDemands.QuantizeFloor(bigTick)
	} else {
		roundedPrice = priceThatMarketDemands.QuantizeFloor(tick)
	}
	skew := priceThatMarketDemands.Sub(roundedPrice)
	priceThatMarketDemands = roundedPrice

	// market orders should fill immediately at price that market demands
	if order.Price.IsZero() {
		order.Price = priceThatMarketDemands
	}

	// determine if this order is able to be filled
	// e.g. order.price is -.5 (debit/buy) would fill when market moves from -.6 to -.4
	// e.g. order.price is +.5 (credit/sell) would fill when market moves from +.4 to +.6
	if priceThatMarketDemands.Cmp(order.Price) < 0 {
		order.Making = true
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
	fillPriceSum := decimal.Zero
	for i, leg := range order.Legs {
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
		// apply quantization skew to last leg so fills sum to order price
		if i == len(order.Legs)-1 {
			if leg.Quantity.IsPositive() {
				fillPrice = fillPrice.Add(skew)
			} else {
				fillPrice = fillPrice.Sub(skew)
			}
		}
		if leg.Quantity.IsPositive() {
			fillPriceSum = fillPriceSum.Sub(fillPrice)
		} else {
			fillPriceSum = fillPriceSum.Add(fillPrice)
		}
		t.Holdings.Add(leg.Security, leg.Quantity, fillPrice)
		leg.Filled = true
	}
	if fillPriceSum.Cmp(priceThatMarketDemands) != 0 {
		panic(fmt.Sprintf("fill price sum %s does not match execution price %s", fillPriceSum, priceThatMarketDemands))
	}

	// simulate commission and regulatory fees
	fee := order.EstimateFee(!order.Making)
	t.Holdings.TotalFees = t.Holdings.TotalFees.Add(fee)

	// caller should now call removePendingOrder if it was added
	return true
}

func (t *Trader) simulateCancelOrder(order *Order) {
	t.removePendingOrder(order)
}
