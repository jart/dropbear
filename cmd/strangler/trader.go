package main

import (
	"dropbear/broker/alpaca"
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
	StateBuyingCall
	StateNeedPut
	StateBuyingPut
	StateReady
)

type Trader struct {
	Config                  *Config
	Chain                   *options.Chain
	Underlying              *options.Equity
	Holdings                Holdings
	OrderEventsSchwab       chan *schwab.OrderEvent
	OrderEventsAlpaca       chan *alpaca.OrderUpdate
	OrderUpdatesSchwab      chan OrderUpdateSchwab
	OptionsByID             map[uint32]*options.Option
	EquitiesByID            map[uint32]*options.Equity
	SecuritiesByName        map[string]options.Security
	OrdersBySchwabID        map[schwab.OrderID]*Order
	OrdersByAlpacaID        map[string]*Order
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
		OrderEventsSchwab:       make(chan *schwab.OrderEvent, 64),
		OrderEventsAlpaca:       make(chan *alpaca.OrderUpdate, 64),
		OrderUpdatesSchwab:      make(chan OrderUpdateSchwab, 64),
		PendingOrders:           map[*Order]bool{},
		PendingOrdersBySecurity: map[options.Security][]*Order{},
		OrdersBySchwabID:        map[schwab.OrderID]*Order{},
		OrdersByAlpacaID:        map[string]*Order{},
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
	if t.Underlying == nil || !t.Underlying.GetBid().IsPositive() || !t.Underlying.GetAsk().IsPositive() {
		return
	}
	switch t.State {
	case StateNeedCall:
		log.Printf("buying call")
		if t.buyCall(now) {
			t.State = StateBuyingCall
		}
	case StateBuyingCall:
		if t.orderCount() == 0 {
			t.State = StateNeedPut
		}
	case StateNeedPut:
		if t.orderCount() != 0 {
			return // wait for call order to fill before buying put
		}
		log.Printf("buying put")
		if t.buyPut(now) {
			t.State = StateBuyingPut
		}
	case StateBuyingPut:
		if t.orderCount() == 0 {
			t.State = StateReady
		}
	case StateReady:
		t.hedgeDelta(now)
	default:
		panic("invalid state")
	}
}

func (t *Trader) orderCount() int {
	if *liveFlag {
		return len(t.OrdersBySchwabID)
	}
	return len(t.PendingOrders)
}

func (t *Trader) pendingOrders() []*Order {
	var orders []*Order
	if *liveFlag {
		for _, order := range t.OrdersBySchwabID {
			orders = append(orders, order)
		}
	} else {
		for order := range t.PendingOrders {
			orders = append(orders, order)
		}
	}
	return orders
}

func (t *Trader) marketOrder(now clocky.Time, security options.Security, quantity decimal.Decimal) {
	t.limitOrder(now, security, quantity, decimal.Zero)
}

func (t *Trader) limitOrder(now clocky.Time, security options.Security, quantity, price decimal.Decimal) {
	t.OrderCounter++
	order := &Order{
		Trader:   t,
		ID:       t.OrderCounter,
		Created:  now,
		Security: security,
		Quantity: quantity,
		Price:    price,
	}
	err := order.Send()
	if err != nil {
		log.Printf("failed to place order: %v\n", err)
	}
}

func (t *Trader) buyCall(now clocky.Time) bool {
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
	t.marketOrder(now, strike.Call, quantity)
	return true
}

func (t *Trader) buyPut(now clocky.Time) bool {
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
	t.marketOrder(now, strike.Put, quantity)
	return true
}

func (t *Trader) hasOrder(orders []*Order) (hasBuy bool, hasSell bool) {
	for _, order := range orders {
		if order.Quantity.IsPositive() {
			hasBuy = true
		} else {
			hasSell = true
		}
	}
	return hasBuy, hasSell
}

func (t *Trader) hedgeDelta(now clocky.Time) {
	orders := t.pendingOrders()
	for _, order := range orders {
		if !order.Canceling && now.After(order.Created.Add(t.Config.Patience)) {
			log.Printf("canceling order #%d after waiting %s\n", order.ID, t.Config.Patience)
			err := order.Cancel()
			if err != nil {
				log.Printf("failed to cancel order #%d: %v\n", order.ID, err)
			}
		}
	}
	price := decimal.Zero
	spread := t.Config.Spread
	bid := t.Underlying.GetBid()
	ask := t.Underlying.GetAsk()
	mid := bid.Add(ask).Half()
	hlf := ask.Sub(bid).Half()
	hasBuy, hasSell := t.hasOrder(orders)
	tolerance := t.Config.Tolerance.Mul(t.Config.Quantum)
	delta := t.computeDelta()
	if !hasBuy && delta.Cmp(tolerance) < 0 {
		// buying: mid + halfSpread * spread
		qty := delta.Neg().QuantizeTruncate(t.Config.Quantum)
		qty = t.clampTradeQuantity(t.Underlying, qty)
		price = mid.Add(hlf.Mul(spread))
		price = price.QuantizeTruncate(decimal.Cent)
		log.Printf("buying %s shares at %s (edge:%s bid:%s ask:%s) to hedge delta of %s\n", qty, price, mid.Sub(price), bid, ask, delta)
		t.limitOrder(now, t.Underlying, qty, price)
	}
	if !hasSell && delta.Neg().Cmp(tolerance) < 0 {
		// selling: mid - halfSpread * spread
		qty := delta.QuantizeTruncate(t.Config.Quantum).Neg()
		qty = t.clampTradeQuantity(t.Underlying, qty)
		price = mid.Sub(hlf.Mul(spread))
		price = price.QuantizeAway(decimal.Cent)
		log.Printf("selling %s shares at %s (edge:%s bid:%s ask:%s) to hedge delta of %s\n", qty.Neg(), price, mid.Sub(price).Neg(), bid, ask, delta)
		t.limitOrder(now, t.Underlying, qty, price)
	}
}

// clampTradeQuantity ensures we don't place an order that would flip our position from long to short or vice versa.
func (t *Trader) clampTradeQuantity(security options.Security, quantity decimal.Decimal) decimal.Decimal {
	holding := t.Holdings.Positions[security]
	if holding == nil {
		return quantity
	}
	pos := holding.Quantity
	if pos.Mul(pos.Add(quantity)).IsNegative() {
		return pos.Neg()
	}
	return quantity
}

func (t *Trader) onHeartbeat() {
	cost := decimal.Zero
	shares := decimal.Zero
	holding := t.Holdings.Positions[t.Underlying]
	if holding != nil {
		shares = holding.Quantity
		cost = holding.AverageCost
	}
	log.Printf("price:%s shares:%s cost:%s delta:%s cash:%s equity:%s orders:%d\n",
		t.Underlying.MidPrice(), shares, cost, t.computeDelta(),
		t.Holdings.Cash, t.Holdings.LiquidationValue(), t.orderCount())
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
	if e.Book.UpdateMBP1(m) {
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
