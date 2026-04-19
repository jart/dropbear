package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
	"dropbear/options"
	"log"
)

type State int

const (
	StateNeedCall State = iota
	StateNeedPut
	StateReady
)

type Trader struct {
	Config                  *Config
	Chain                   *options.Chain
	Underlying              *options.Equity
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

func NewTrader(config *Config) *Trader {
	return &Trader{
		Config:                  config,
		Chain:                   options.NewChain(),
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
	if len(t.PendingOrdersBySecurity) != 0 || len(t.OrdersBySchwabID) != 0 {
		return // don't run when there's pending orders
	}
	if t.Underlying == nil || !t.Underlying.Bid.IsPositive() || !t.Underlying.Ask.IsPositive() {
		return
	}
	switch t.State {
	case StateNeedCall:
		log.Printf("buying call")
		if t.buyCall() {
			t.State = StateNeedPut
		}
	case StateNeedPut:
		log.Printf("buying put")
		if t.buyPut() {
			t.State = StateReady
		}
	case StateReady:
		t.hedgeDelta()
	}
}

func (t *Trader) marketOrder(security options.Security, quantity decimal.Decimal) {
	t.OrderCounter++
	order := &Order{
		Trader:  t,
		ID:      t.OrderCounter,
		Created: clocky.Now(),
		Legs: []*Leg{{
			Security: security,
			Quantity: quantity,
		}},
	}
	order.Send()
}

func (t *Trader) buyCall() bool {
	strike := t.Chain.AtTheMoney
	n := t.Config.Strikes / 2
	for n > 0 {
		if strike == nil || !strike.IsReady() {
			return false
		}
		strike = strike.Next
		n--
	}
	if !strike.Call.HasGreeks() {
		return false
	}
	quantity := t.Config.Straddles.Mul(t.Config.Direction)
	t.marketOrder(strike.Call, quantity)
	return true
}

func (t *Trader) buyPut() bool {
	strike := t.Chain.AtTheMoney
	n := (t.Config.Strikes + 1) / 2
	for n > 0 {
		if strike == nil || !strike.IsReady() {
			return false
		}
		strike = strike.Prev
		n--
	}
	if !strike.Put.HasGreeks() {
		return false
	}
	quantity := t.Config.Straddles.Mul(t.Config.Direction)
	t.marketOrder(strike.Put, quantity)
	return true
}

func (t *Trader) hedgeDelta() {
	delta := t.computeDelta()
	qty := delta.QuantizeTruncate(t.Config.Quantum).Neg()
	if qty.IsZero() {
		return
	}
	t.onHeartbeat()
	log.Printf("buying %s shares to hedge delta of %s\n", qty, delta)
	t.marketOrder(t.Underlying, qty)
}

func (t *Trader) onHeartbeat() {
	cost := decimal.Zero
	shares := decimal.Zero
	holding := t.Holdings.Positions[t.Underlying]
	if holding != nil {
		shares = holding.Quantity
		cost = holding.AverageCost
	}
	log.Printf("price:%s shares:%s cost:%s delta:%s cash:%s equity:%s",
		t.Underlying.MidPrice(), shares, cost, t.computeDelta(),
		t.Holdings.Cash, t.Holdings.LiquidationValue())
}

func (t *Trader) computeDelta() decimal.Decimal {
	delta := decimal.Zero
	for security, holding := range t.Holdings.Positions {
		delta = delta.Add(security.GetDelta().Mul(holding.Quantity).MulInt(security.Multiplier()))
	}
	return delta
}

func (t *Trader) onEndOfDay() {
	balance := t.Holdings.LiquidationValue()
	totalFees := t.Holdings.TotalFees
	log.Printf("ending balance %s less %s fees winning %s",
		balance, totalFees, balance.Sub(totalFees))
}

func (t *Trader) onOptionDef(o *options.Option) {
	name := o.Name()
	if existing := t.SecuritiesByName[name]; existing != nil {
		o2 := existing.(*options.Option)
		o2.ID = o.ID
		o = o2
	}
	t.OptionsByID[o.GetID()] = o
	t.SecuritiesByName[name] = o
}

func (t *Trader) onEquityDef(e *options.Equity) {
	name := e.Name()
	if existing := t.SecuritiesByName[name]; existing != nil {
		e2 := existing.(*options.Equity)
		e2.ID = e.ID
		e = e2
	}
	t.EquitiesByID[e.GetID()] = e
	t.SecuritiesByName[name] = e
	if e.Symbol == t.Config.Symbol {
		t.Underlying = e
	}
}

func (t *Trader) onDefEnd() {
	now := clocky.Now()
	year, month, day := now.Date()
	t.MarketClose = cboe.GetCloseTime(year, month, day)
}

func (t *Trader) onEquityTick(m *databento.MBP1) *options.Equity {
	e := t.EquitiesByID[m.Header.InstrumentID]
	if e != nil {
		switch m.Action {
		case databento.ActionTrade:
			t.onEquityTrade(e, m)
		case databento.ActionAdd, databento.ActionCancel, databento.ActionClear, databento.ActionModify:
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
	mustRecomputeGreeks := false
	if bid != databento.UndefPrice {
		price := decimal.Decimal(bid / 1000)
		if price.Cmp(e.Bid) != 0 {
			mustRecomputeGreeks = true
		}
		e.Bid = price
		e.BidSize = m.Levels[0].BidSz
	} else {
		e.Bid = decimal.Zero
		e.BidSize = 0
	}
	ask := m.Levels[0].AskPx
	if ask != databento.UndefPrice {
		price := decimal.Decimal(ask / 1000)
		if price.Cmp(e.Ask) != 0 {
			mustRecomputeGreeks = true
		}
		e.Ask = price
		e.AskSize = m.Levels[0].AskSz
	} else {
		e.Ask = decimal.Zero
		e.AskSize = 0
	}
	if mustRecomputeGreeks {
		for _, holding := range t.Holdings.Positions {
			if o, ok := holding.Security.(*options.Option); ok && o.Symbol == e.Symbol {
				o.ComputeGreeks(e.MidPrice(), kRiskFreeRate)
			}
		}
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
	mustRecomputeGreeks = true
	if mustRecomputeGreeks && t.Underlying != nil {
		o.ComputeGreeks(t.Underlying.MidPrice(), kRiskFreeRate)
	}
}
