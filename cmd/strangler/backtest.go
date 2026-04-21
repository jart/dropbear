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
			t.simulateFills(now, e)
		case *databento.CMBP1:
			now = m.TSRecv
			clocky.SetNow(now)
			o := t.onOptionTick(m)
			if o == nil {
				continue
			}
			t.simulateFills(now, o)
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
			}
			t.onThought(now)
			if now.After(nextHeartbeat) {
				nextHeartbeat = now.Add(*heartbeatFlag)
				t.onHeartbeat()
				if t.Web != nil {
					t.Web.broadcastState()
				}
			}
		}
	}
	t.onEndOfDay()
	return nil
}

func (t *Trader) simulateOrder(order *Order) {
	t.addPendingOrder(order)
}

func (t *Trader) simulateFills(now clocky.Time, security options.Security) {
	for _, order := range slices.Clone(t.PendingOrdersBySecurity[security]) {
		if t.simulateFillOrder(now, order) {
			t.removePendingOrder(order)
		}
	}
}

func (t *Trader) simulateFillOrder(now clocky.Time, order *Order) bool {
	if order.Filled {
		panic("order already filled")
	}
	if now.Before(order.Created.Add(*latencyFlag)) {
		return false
	}
	switch e := order.Security.(type) {
	case *options.Option:
		var demand decimal.Decimal
		if order.Quantity.IsPositive() {
			demand = e.Ask
			if !order.Price.IsZero() && order.Price.Cmp(demand) < 0 {
				order.Making = true
				return false // market hasn't reached our limit yet
			}
		} else {
			demand = e.Bid
			if !order.Price.IsZero() && order.Price.Cmp(demand) > 0 {
				order.Making = true
				return false // market hasn't reached our limit yet
			}
		}
		log.Printf("order #%d transacted %s options at price %s (limit %s)\n",
			order.ID, order.Quantity, demand, order.Price)
		t.Holdings.Add(order.Security, order.Quantity, demand)
		order.Filled = true
		fee := order.EstimateFee(order.Quantity, true, !order.Making)
		t.Holdings.TotalFees = t.Holdings.TotalFees.Add(fee)
		return true
	case *options.Equity:
		got := decimal.Zero
		need := order.Unfilled
		first := need.Cmp(order.Quantity.Abs()) == 0
		if order.Quantity.IsPositive() {
			// let's buy stock
			for {
				if len(e.Book.Asks.Levels) == 0 {
					break // no more asks, market hasn't reached our limit yet
				}
				level := e.Book.Asks.Levels[len(e.Book.Asks.Levels)-1]
				if !order.Price.IsZero() && order.Price.Cmp(level.Price) < 0 {
					break // market hasn't reached our limit yet
				}
				take := level.Size.Min(need.Sub(got))
				if take.Cmp(level.Size) == 0 {
					e.Book.Asks.Levels = e.Book.Asks.Levels[:len(e.Book.Asks.Levels)-1]
				} else {
					e.Book.Asks.Levels[len(e.Book.Asks.Levels)-1].Size = level.Size.Sub(take)
				}
				got = got.Add(take)
				t.Holdings.Add(order.Security, take, level.Price)
				log.Printf("order #%d bought %s at price %s (limit %s) after %s\n",
					order.ID, got, level.Price, order.Price, now.Sub(order.Created))
				if got.Cmp(need) == 0 {
					break
				}
			}
		} else {
			// let's sell stock
			for {
				if len(e.Book.Bids.Levels) == 0 {
					break // no more bids, market hasn't reached our limit yet
				}
				level := e.Book.Bids.Levels[len(e.Book.Bids.Levels)-1]
				if !order.Price.IsZero() && order.Price.Cmp(level.Price) > 0 {
					break // market hasn't reached our limit yet
				}
				take := level.Size.Min(need.Sub(got))
				if take.Cmp(level.Size) == 0 {
					e.Book.Bids.Levels = e.Book.Bids.Levels[:len(e.Book.Bids.Levels)-1]
				} else {
					e.Book.Bids.Levels[len(e.Book.Bids.Levels)-1].Size = level.Size.Sub(take)
				}
				got = got.Add(take)
				t.Holdings.Add(order.Security, take.Neg(), level.Price)
				log.Printf("order #%d sold %s at price %s (limit %s) after %s\n",
					order.ID, got, level.Price, order.Price, now.Sub(order.Created))
				if got.Cmp(need) == 0 {
					break
				}
			}
		}
		if first && got.Cmp(need) != 0 {
			order.Making = true
		}
		if got.IsZero() {
			return false // market hasn't reached our limit yet
		}
		log.Printf("order #%d filled %s of %s after %s\n", order.ID, got, need, now.Sub(order.Created))
		order.Unfilled = need.Sub(got)
		if got.Cmp(need) == 0 {
			order.Filled = true
		}
		fee := order.EstimateFee(got, first, !order.Making)
		t.Holdings.TotalFees = t.Holdings.TotalFees.Add(fee)
		return order.Filled
	default:
		panic("unsupported security type")
	}
}

func (t *Trader) simulateCancelOrder(order *Order) {
	t.removePendingOrder(order)
}
