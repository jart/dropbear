package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
	"dropbear/options"
	"dropbear/symbol"
	"fmt"
	"log"
)

type State int

const (
	StateInit State = iota
	StateReady
)

type Trader struct {
	Symbol                  symbol.Symbol
	Config                  *Config
	Chain                   *options.Options
	Holdings                Holdings
	OrderEvents             chan *schwab.OrderEvent
	OrderUpdates            chan OrderUpdate
	OptionsByID             map[uint32]*options.Option
	EquitiesByID            map[uint32]*options.Equity
	SecuritiesByName        map[string]options.Security
	OrdersBySchwabID        map[schwab.OrderID]*Order
	PendingOrders           map[*Order]bool
	PendingOrdersBySecurity map[options.Security][]*Order
	MarketClose             clocky.Time
	Hinter                  *loggy.Hinter
	State                   State
	OrderCounter            int
}

func NewTrader(symbol symbol.Symbol, config *Config) *Trader {
	return &Trader{
		Symbol:                  symbol,
		Config:                  config,
		Chain:                   options.NewOptions(),
		Holdings:                Holdings{Positions: map[options.Security]*Holding{}},
		OrderEvents:             make(chan *schwab.OrderEvent, 64),
		OrderUpdates:            make(chan OrderUpdate, 64),
		PendingOrders:           map[*Order]bool{},
		PendingOrdersBySecurity: map[options.Security][]*Order{},
		OrdersBySchwabID:        map[schwab.OrderID]*Order{},
		OptionsByID:             map[uint32]*options.Option{},
		EquitiesByID:            map[uint32]*options.Equity{},
		SecuritiesByName:        map[string]options.Security{},
		Hinter:                  loggy.NewHinter(),
	}
}

func (t *Trader) onThought(now clocky.Time) {
	clock := now.ClockInt()
	if clock < t.Config.StartOfDay {
		t.Hinter.Hint("not trading: before market open (%d < %d)", clock, t.Config.StartOfDay)
		return
	}
	if now.After(t.MarketClose) {
		t.Hinter.Hint("not trading: market closed")
		return
	}
	if len(t.PendingOrdersBySecurity) != 0 {
		return
	}
	switch t.State {
	case StateInit:
		log.Printf("buying straddle")
		t.buyCall()
		t.buyPut()
		t.State = StateReady
	case StateReady:
		t.hedgeDelta()
	}
}

func (t *Trader) buyCall() {
	atm := t.Chain.AtTheMoney
	if atm == nil || !atm.IsReady() || !atm.Call.Ask.IsPositive() {
		return
	}
	t.OrderCounter++
	order := &Order{
		Trader: t,
		ID:     t.OrderCounter,
		Legs:   []*Leg{{Security: atm.Call, Quantity: t.Config.Straddles}},
	}
	order.Send()
}

func (t *Trader) buyPut() {
	atm := t.Chain.AtTheMoney
	if atm == nil || !atm.IsReady() || !atm.Put.Ask.IsPositive() {
		return
	}
	t.OrderCounter++
	order := &Order{
		Trader: t,
		ID:     t.OrderCounter,
		Legs:   []*Leg{{Security: atm.Put, Quantity: t.Config.Straddles}},
	}
	order.Send()
}

func (t *Trader) hedgeDelta() {
	equity := t.SecuritiesByName[t.Symbol.String()]
	if equity == nil || !equity.GetBid().IsPositive() || !equity.GetAsk().IsPositive() {
		return
	}
	delta := t.computeDelta()
	qty := delta.QuantizeTruncate(t.Config.Quantum).Neg()
	if qty.IsZero() {
		return
	}
	log.Printf("buying %s shares to hedge delta of %s\n", qty, delta)
	order := &Order{
		Trader: t,
		ID:     t.OrderCounter,
		Legs:   []*Leg{{Security: equity, Quantity: qty}},
	}
	order.Send()
}

func (t *Trader) onHeartbeat() {
	equity := t.SecuritiesByName[t.Symbol.String()]
	if equity == nil {
		return
	}
	shares := decimal.Zero
	if holding := t.Holdings.Positions[equity]; holding != nil {
		shares = holding.Quantity
	}
	log.Printf("price:%s shares:%s delta:%s cash:%s equity:%s",
		equity.MidPrice(), shares, t.computeDelta(), t.Holdings.Cash, t.Holdings.LiquidationValue())
}

func (t *Trader) computeDelta() decimal.Decimal {
	delta := decimal.Zero
	for security, holding := range t.Holdings.Positions {
		delta = delta.Add(security.GetDelta().Mul(holding.Quantity).MulInt(security.Multiplier()))
	}
	return delta
}

func (t *Trader) onEndOfDay() {
	fmt.Printf("%s\n", t.Holdings.LiquidationValue())
}

func (t *Trader) onOptionDef(o *options.Option) {
	name := o.GetName()
	if existing := t.SecuritiesByName[name]; existing != nil {
		o2 := existing.(*options.Option)
		o2.ID = o.ID
		o = o2
	}
	t.OptionsByID[o.GetID()] = o
	t.SecuritiesByName[name] = o
}

func (t *Trader) onEquityDef(e *options.Equity) {
	name := e.GetName()
	if existing := t.SecuritiesByName[name]; existing != nil {
		e2 := existing.(*options.Equity)
		e2.ID = e.ID
		e = e2
	}
	t.EquitiesByID[e.GetID()] = e
	t.SecuritiesByName[name] = e
}

func (t *Trader) onOptionDefEnd() {

	// get close time
	now := clocky.Now()
	year, month, day := now.Date()
	t.MarketClose = cboe.GetCloseTime(year, month, day)

	// initialize options restrictions after loading schwab order history
	for security, holding := range t.Holdings.Positions {
		if option := security.(*options.Option); option != nil {
			if holding.Quantity.IsPositive() {
				option.Mode = options.ModeLong
			} else if holding.Quantity.IsNegative() {
				option.Mode = options.ModeShort
			}
		}
	}

	// ensure puts and calls at same strike have opposite restriction
	for it := t.Chain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		switch strike.Call.Mode {
		case options.ModeNone:
			if strike.Put.Mode != options.ModeNone {
				strike.Call.Mode = strike.Put.Mode.Invert()
			}
		case options.ModeLong:
			if strike.Put.Mode == options.ModeNone {
				strike.Put.Mode = options.ModeShort
			}
		case options.ModeShort:
			if strike.Put.Mode == options.ModeNone {
				strike.Put.Mode = options.ModeLong
			}
		}
	}

	// ensure at-the-money has a restriction
	if t.Chain.AtTheMoney.Call.Mode == options.ModeNone {
		t.Chain.AtTheMoney.Call.Mode = options.ModeShort
		t.Chain.AtTheMoney.Put.Mode = options.ModeLong
	}

	// now color remaining strikes as checkered as possible
	for s := t.Chain.AtTheMoney.Next; s != nil; s = s.Next {
		if s.Call.Mode == options.ModeNone {
			s.Call.Mode = s.Prev.Call.Mode.Invert()
		}
		if s.Put.Mode == options.ModeNone {
			s.Put.Mode = s.Prev.Put.Mode.Invert()
		}
	}
	for s := t.Chain.AtTheMoney.Prev; s != nil; s = s.Prev {
		if s.Call.Mode == options.ModeNone {
			s.Call.Mode = s.Next.Call.Mode.Invert()
		}
		if s.Put.Mode == options.ModeNone {
			s.Put.Mode = s.Next.Put.Mode.Invert()
		}
	}
}

func (t *Trader) onEquityTick(m *databento.MBP1) *options.Equity {
	e := t.EquitiesByID[m.Header.InstrumentID]
	if e != nil {
		switch m.Action {
		case databento.ActionTrade:
			t.onEquityTrade(e, m)
		case databento.ActionAdd, databento.ActionCancel:
			if m.Header.TSEvent > e.TS {
				t.onEquityQuote(e, m)
			}
		default:
			panic("unexpected equity tick action: " + m.Action.String())
		}
	}
	return e
}

func (t *Trader) onEquityTrade(e *options.Equity, m *databento.MBP1) {
}

func (t *Trader) onEquityQuote(e *options.Equity, m *databento.MBP1) {
	e.TS = m.Header.TSEvent
	bid := m.Levels[0].BidPx
	if bid != databento.UndefPrice {
		price := decimal.Decimal(bid / 1000)
		e.Bid = price
		e.BidSize = m.Levels[0].BidSz
	} else {
		e.Bid = decimal.Zero
		e.BidSize = 0
	}
	ask := m.Levels[0].AskPx
	if ask != databento.UndefPrice {
		price := decimal.Decimal(ask / 1000)
		e.Ask = price
		e.AskSize = m.Levels[0].AskSz
	} else {
		e.Ask = decimal.Zero
		e.AskSize = 0
	}
}

func (t *Trader) onOptionTick(m *databento.CMBP1) *options.Option {
	o := t.OptionsByID[m.Header.InstrumentID]
	if o != nil {
		switch m.Action {
		case databento.ActionTrade:
			t.onOptionTrade(o, m)
		case databento.ActionAdd:
			if m.Header.TSEvent > o.TS {
				t.onOptionQuote(o, m)
			}
		default:
			panic("unexpected option tick action: " + m.Action.String())
		}
	}
	return o
}

func (t *Trader) onOptionTrade(o *options.Option, m *databento.CMBP1) {
}

func (t *Trader) onOptionQuote(o *options.Option, m *databento.CMBP1) {
	o.TS = m.Header.TSEvent
	mustRecomputeGreeks := false
	bid := m.Levels[0].BidPx
	if bid != databento.UndefPrice {
		price := decimal.Decimal(bid / 1000)
		if price.Cmp(o.Bid) != 0 {
			mustRecomputeGreeks = true
		}
		o.Bid = price
		o.BidSize = m.Levels[0].BidSz
		o.Got |= options.GotBid
	} else {
		o.Bid = decimal.Zero
		o.BidSize = 0
		o.Got &^= options.GotBid
	}
	ask := m.Levels[0].AskPx
	if ask != databento.UndefPrice {
		price := decimal.Decimal(ask / 1000)
		if price.Cmp(o.Ask) != 0 {
			mustRecomputeGreeks = true
		}
		o.Ask = price
		o.AskSize = m.Levels[0].AskSz
		o.Got |= options.GotAsk
	} else {
		o.Ask = decimal.Zero
		o.AskSize = 0
		o.Got &^= options.GotAsk
	}
	if t.Chain.Add(o) {
		mustRecomputeGreeks = true
	}
	if mustRecomputeGreeks {
		equity := t.SecuritiesByName[o.Symbol.String()]
		if equity != nil && equity.MidPrice().IsPositive() {
			o.ComputeGreeks(t.Chain.Price, kRiskFreeRate, equity.MidPrice())
		}
	}
}
