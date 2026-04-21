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
	"fmt"
	"log"
)

type State int

const (
	StateWingCall State = iota
	StateWingCall2
	StateWingPut
	StateWingPut2
	StateStrangleCall
	StateStrangleCall2
	StateStranglePut
	StateStranglePut2
	StateReady
)

func (s State) String() string {
	switch s {
	case StateWingCall:
		return "buying wing call"
	case StateWingCall2:
		return "filling wing call"
	case StateWingPut:
		return "buying wing put"
	case StateWingPut2:
		return "filling wing put"
	case StateStrangleCall:
		return "opening call"
	case StateStrangleCall2:
		return "filling call"
	case StateStranglePut:
		return "opening put"
	case StateStranglePut2:
		return "filling put"
	case StateReady:
		return "hedging"
	default:
		return "unknown"
	}
}

type Trader struct {
	Config                  *Config
	Chain                   *options.Chain
	Underlying              *options.Equity
	Holdings                Holdings
	Web                     *Web
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
	Paused                  bool
}

func NewTrader(config *Config) *Trader {
	return &Trader{
		Config:                  config,
		Web:                     NewWeb(),
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
	if t.Paused {
		t.Hinter.Hint("not trading: paused")
		return
	}
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
	// either buy a strangle or sell an iron condor
	// we open one leg at a time so we don't have to support multi-leg orders
	// for short strangles we buy the wings first to reduce our margin requirements
	switch t.State {
	case StateWingCall:
		if t.Config.Direction.IsPositive() {
			// we don't need wings on a long strangle
			t.State = StateStrangleCall
			break
		}
		if t.Config.Wing.IsZero() {
			// you better have plenty of margin
			t.State = StateStrangleCall
			break
		}
		if t.buyWingCall(now) {
			t.State = StateWingCall2
		}
	case StateWingCall2:
		if t.orderCount() == 0 {
			t.State = StateWingPut
		}
	case StateWingPut:
		if t.buyWingPut(now) {
			t.State = StateWingPut2
		}
	case StateWingPut2:
		if t.orderCount() == 0 {
			t.State = StateStrangleCall
		}
	case StateStrangleCall:
		if t.openStrangleCall(now) {
			t.State = StateStrangleCall2
		}
	case StateStrangleCall2:
		if t.orderCount() == 0 {
			t.State = StateStranglePut
		}
	case StateStranglePut:
		if t.openStranglePut(now) {
			t.State = StateStranglePut2
		}
	case StateStranglePut2:
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

// buyWingCall buys the cheapest call to reduce margin requirement when -direction=-1
// this effectively turns the short strangle into a short iron condor
func (t *Trader) buyWingCall(now clocky.Time) bool {
	strike := t.Chain.AtTheMoney
	for {
		if strike == nil {
			t.Hinter.Hint("ran out of strikes")
			return false
		}
		if !strike.IsReady() {
			t.Hinter.Hint("strike %s not ready", strike)
			return false
		}
		if !strike.Call.HasGreeks() {
			t.Hinter.Hint("greeks not ready yet for strike %s underlying is %s", strike, t.Underlying.MidPrice())
			return false
		}
		if strike.Call.Ask.IsPositive() && strike.Call.Ask.Cmp(t.Config.Wing) <= 0 {
			break
		}
		strike = strike.Next
	}
	quantity := t.Config.Straddles
	log.Printf("we shall buy %s wing call at strike %s with ask price %s underlying is %s\n", quantity, strike, strike.Call.Ask, t.Underlying.MidPrice())
	t.marketOrder(now, strike.Call, quantity)
	return true
}

// buyWingPut buys the cheapest put to reduce margin requirement when -direction=-1
// this effectively turns the short strangle into a short iron condor
func (t *Trader) buyWingPut(now clocky.Time) bool {
	strike := t.Chain.AtTheMoney
	for {
		if strike == nil {
			t.Hinter.Hint("ran out of strikes")
			return false
		}
		if !strike.IsReady() {
			t.Hinter.Hint("strike %s not ready", strike)
			return false
		}
		if !strike.Put.HasGreeks() {
			t.Hinter.Hint("greeks not ready yet for strike %s underlying is %s", strike, t.Underlying.MidPrice())
			return false
		}
		if strike.Put.Ask.IsPositive() && strike.Put.Ask.Cmp(t.Config.Wing) <= 0 {
			break
		}
		strike = strike.Prev
	}
	quantity := t.Config.Straddles
	log.Printf("we shall buy %s wing put at strike %s with ask price %s underlying is %s\n", quantity, strike, strike.Put.Ask, t.Underlying.MidPrice())
	t.marketOrder(now, strike.Put, quantity)
	return true
}

func (t *Trader) openStrangleCall(now clocky.Time) bool {
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
	log.Printf("we shall trade %s strangle call at strike %s with ask price %s underlying is %s\n", quantity, strike, strike.Call.Ask, t.Underlying.MidPrice())
	t.marketOrder(now, strike.Call, quantity)
	return true
}

func (t *Trader) openStranglePut(now clocky.Time) bool {
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
	log.Printf("we shall trade %s strangle put at strike %s with ask price %s underlying is %s\n", quantity, strike, strike.Put.Ask, t.Underlying.MidPrice())
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
	qty := decimal.Zero
	price := decimal.Zero
	spread := t.Config.Spread
	bid := t.Underlying.GetBid()
	ask := t.Underlying.GetAsk()
	mid := bid.Add(ask).Half()
	hlf := ask.Sub(bid).Half()
	hasBuy, hasSell := t.hasOrder(orders)
	tolerance := t.Config.Tolerance.Mul(t.Config.Quantum)
	holding := t.Holdings.Positions[t.Underlying]
	delta := t.computeDelta()
	if !hasBuy && delta.Cmp(tolerance) < 0 {
		if delta.IsNegative() {
			// we genuinely need to buy in order to hedge
			// therefore we must be willing to buy as much as possible
			qty = delta.Neg().QuantizeTruncate(t.Config.Quantum)
		} else {
			// we're market making (because tolerance is positive)
			// therefore let's not trade any more than a round lot
			qty = t.Config.Quantum
		}
		qty = t.clampTradeQuantity(t.Underlying, qty)
		if !qty.IsZero() {
			// buying: mid + halfSpread * spread
			price = mid.Add(hlf.Mul(spread))
			price = price.QuantizeTruncate(decimal.Cent)
			extra := ""
			if holding != nil && holding.Quantity.IsNegative() {
				gain := holding.AverageCost.Sub(price)
				extra = fmt.Sprintf(" to realize gain %s", gain.Mul(qty))
			}
			log.Printf("buying %s shares at %s (edge:%s bid:%s ask:%s) to hedge delta of %s%s\n", qty, price, mid.Sub(price), bid, ask, delta, extra)
			t.limitOrder(now, t.Underlying, qty, price)
		}
	}
	if !hasSell && delta.Neg().Cmp(tolerance) < 0 {
		if delta.IsPositive() {
			// we genuinely need to sell in order to hedge
			// therefore we must be willing to sell as much as possible
			qty = delta.QuantizeTruncate(t.Config.Quantum).Neg()
		} else {
			// we're market making (because tolerance is positive)
			// therefore let's not trade any more than a round lot
			qty = t.Config.Quantum.Neg()
		}
		qty = t.clampTradeQuantity(t.Underlying, qty)
		if !qty.IsZero() {
			// selling: mid - halfSpread * spread
			price = mid.Sub(hlf.Mul(spread))
			price = price.QuantizeAway(decimal.Cent)
			extra := ""
			if holding != nil && holding.Quantity.IsPositive() {
				gain := price.Sub(holding.AverageCost)
				extra = fmt.Sprintf(" to realize gain %s", gain.Mul(qty.Neg()))
			}
			log.Printf("selling %s shares at %s (edge:%s bid:%s ask:%s) to hedge delta of %s%s\n", qty.Neg(), price, mid.Sub(price), bid, ask, delta, extra)
			t.limitOrder(now, t.Underlying, qty, price)
			t.onHeartbeat()
		}
	}
}

// clampTradeQuantity ensures we don't place an order that would flip our position from long to short or vice versa.
func (t *Trader) clampTradeQuantity(security options.Security, quantity decimal.Decimal) decimal.Decimal {
	holding := t.Holdings.Positions[security]
	if holding == nil {
		return quantity
	}
	if holding.Quantity.IsZero() {
		return quantity
	} else if holding.Quantity.IsPositive() {
		if quantity.IsPositive() {
			// we're long and buying more
			return quantity
		} else {
			// we're long and selling
			if quantity.Neg().Cmp(holding.Quantity) > 0 {
				return holding.Quantity.Neg()
			}
		}
	} else {
		if quantity.IsNegative() {
			// we're short and selling more
			return quantity
		} else {
			// we're short and buying
			if quantity.Cmp(holding.Quantity.Neg()) > 0 {
				return holding.Quantity.Neg()
			}
		}
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
