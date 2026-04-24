package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/indicators"
	"dropbear/loggy"
	"dropbear/options"
	"fmt"
	"log"
)

type Transaction struct {
	Time     clocky.Time
	Security options.Security
	Quantity decimal.Decimal
	Price    decimal.Decimal
	OrderID  int
}

type Metrics struct {

	// these indicators might help know when to buy/sell options
	// by default they have hours of lookback at tick granularity
	MinIV *indicators.Min
	MaxIV *indicators.Max

	// these indicators might help us smooth out noise in quote prices
	// by default they have thirteen samples of lookback
	BidMA *indicators.WWMA
	AskMA *indicators.WWMA
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
	FailedOrders            chan *Order
	OptionsByID             map[uint32]*options.Option
	EquitiesByID            map[uint32]*options.Equity
	SecuritiesByName        map[string]options.Security
	OrdersBySchwabID        map[schwab.OrderID]*Order
	OrdersByClientOrderID   map[string]*Order
	PendingOrders           map[*Order]bool
	Metrics                 map[options.Security]*Metrics
	PendingOrdersBySecurity map[options.Security][]*Order
	MarketClose             clocky.Time
	Transactions            []*Transaction
	Hinter                  *loggy.Hinter
	NextHedge               clocky.Time
	OrderCounter            int
	Paused                  bool
}

func NewTrader(config *Config) *Trader {
	t := &Trader{
		Config:                  config,
		Web:                     NewWeb(),
		Chain:                   options.NewChain(),
		Holdings:                Holdings{Positions: map[options.Security]*Holding{}},
		OrderEventsSchwab:       make(chan *schwab.OrderEvent, 64),
		OrderEventsAlpaca:       make(chan *alpaca.OrderUpdate, 64),
		OrderUpdatesSchwab:      make(chan OrderUpdateSchwab, 64),
		FailedOrders:            make(chan *Order, 64),
		PendingOrders:           map[*Order]bool{},
		PendingOrdersBySecurity: map[options.Security][]*Order{},
		OrdersBySchwabID:        map[schwab.OrderID]*Order{},
		OrdersByClientOrderID:   map[string]*Order{},
		OptionsByID:             map[uint32]*options.Option{},
		EquitiesByID:            map[uint32]*options.Equity{},
		SecuritiesByName:        map[string]options.Security{},
		Metrics:                 map[options.Security]*Metrics{},
		Hinter:                  loggy.NewHinter(),
	}
	t.Web.Trader = t
	return t
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
	t.manageStrangles(now)
	t.hedgeDelta(now)
}

func (t *Trader) orderCount() int {
	return len(t.PendingOrders)
}

func (t *Trader) pendingOrders() []*Order {
	var orders []*Order
	for order := range t.PendingOrders {
		orders = append(orders, order)
	}
	return orders
}

func (t *Trader) order(now clocky.Time, security options.Security, quantity decimal.Decimal) *Order {
	return t.limitOrder(now, security, quantity, t.getMidpointPrice(security, quantity))
}

func (t *Trader) limitOrder(now clocky.Time, security options.Security, quantity, price decimal.Decimal) *Order {
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
	return order
}

func (t *Trader) getMidpointPrice(security options.Security, quantity decimal.Decimal) decimal.Decimal {
	mid := security.MidPrice()
	tick, bigTick := security.Ticks()
	if mid.Cmp(decimal.Three) > 0 {
		tick = bigTick
	}
	if quantity.IsPositive() {
		return mid.QuantizeTruncate(tick) // buy low
	} else {
		return mid.QuantizeAway(tick) // sell high
	}
}

// finds a call very far out of the money to make margin go down when we sell calls
func (t *Trader) findWingCall() *options.Option {
	strike := t.Chain.AtTheMoney
	for {
		if strike == nil {
			return nil
		}
		if !strike.IsReady() {
			return nil
		}
		if !strike.Call.Ask.IsPositive() {
			return nil
		}
		if strike.Call.Ask.Cmp(t.Config.Wing) <= 0 {
			break
		}
		strike = strike.Next
	}
	return strike.Call
}

// finds a put very far out of the money to make margin go down when we sell puts
func (t *Trader) findWingPut() *options.Option {
	strike := t.Chain.AtTheMoney
	for {
		if strike == nil {
			return nil
		}
		if !strike.IsReady() {
			return nil
		}
		if !strike.Put.Ask.IsPositive() {
			return nil
		}
		if strike.Put.Ask.Cmp(t.Config.Wing) <= 0 {
			break
		}
		strike = strike.Prev
	}
	return strike.Put
}

// finds a call slightly out of the money
func (t *Trader) findCall() *options.Option {
	strike := t.Chain.AtTheMoney
	n := t.Config.Strikes / 2
	for n > 0 {
		if strike == nil || !strike.IsReady() {
			return nil
		}
		strike = strike.Next
		n--
	}
	if !strike.Call.HasGreeks() {
		return nil
	}
	return strike.Call
}

// finds a put slightly out of the money
func (t *Trader) findPut() *options.Option {
	strike := t.Chain.AtTheMoney
	n := (t.Config.Strikes + 1) / 2
	for n > 0 {
		if strike == nil || !strike.IsReady() {
			return nil
		}
		strike = strike.Prev
		n--
	}
	if !strike.Put.HasGreeks() {
		return nil
	}
	return strike.Put
}

// manageStrangles builds strangle positions one leg at a time.
// For selling (direction=-1): buys wings first, then sells when IV is high.
// For buying (direction=+1): buys when IV is low.
func (t *Trader) manageStrangles(now clocky.Time) {
	if t.Config.Direction.IsZero() {
		return
	}
	if t.hasPendingOptionOrder() {
		return
	}
	shortCalls, shortPuts, longCalls, longPuts := t.countOptionLegs()
	limit := t.Config.Straddles.Int()
	if t.Config.Direction.IsNegative() {
		t.manageSellStrangles(now, shortCalls, shortPuts, longCalls, longPuts, limit)
	} else {
		t.manageBuyStrangles(now, longCalls, longPuts, limit)
	}
}

func (t *Trader) manageSellStrangles(now clocky.Time, shortCalls, shortPuts, longCalls, longPuts, limit int) {
	if shortCalls < limit {
		if !t.Config.Wing.IsZero() && longCalls <= shortCalls {
			if wing := t.findWingCall(); wing != nil {
				log.Printf("buying wing call %s (wings:%d short:%d)\n", wing.Name(), longCalls, shortCalls)
				t.order(now, wing, decimal.One)
			} else {
				t.Hinter.Hint("can't find wing call")
			}
			return
		}
		if call := t.findCall(); call != nil && t.isIVFavorable(call) {
			log.Printf("selling call %s (iv:%s short:%d/%d)\n", call.Name(), call.IV, shortCalls+1, limit)
			t.order(now, call, decimal.NegOne)
			return
		}
	}
	if shortPuts < limit {
		if !t.Config.Wing.IsZero() && longPuts <= shortPuts {
			if wing := t.findWingPut(); wing != nil {
				log.Printf("buying wing put %s (wings:%d short:%d)\n", wing.Name(), longPuts, shortPuts)
				t.order(now, wing, decimal.One)
			} else {
				t.Hinter.Hint("can't find wing put")
			}
			return
		}
		if put := t.findPut(); put != nil && t.isIVFavorable(put) {
			log.Printf("selling put %s (iv:%s short:%d/%d)\n", put.Name(), put.IV, shortPuts+1, limit)
			t.order(now, put, decimal.NegOne)
			return
		}
	}
}

func (t *Trader) manageBuyStrangles(now clocky.Time, longCalls, longPuts, limit int) {
	if longCalls < limit {
		if call := t.findCall(); call != nil && t.isIVFavorable(call) {
			log.Printf("buying call %s (iv:%s long:%d/%d)\n", call.Name(), call.IV, longCalls+1, limit)
			t.order(now, call, decimal.One)
			return
		}
	}
	if longPuts < limit {
		if put := t.findPut(); put != nil && t.isIVFavorable(put) {
			log.Printf("buying put %s (iv:%s long:%d/%d)\n", put.Name(), put.IV, longPuts+1, limit)
			t.order(now, put, decimal.One)
			return
		}
	}
}

func (t *Trader) hasPendingOptionOrder() bool {
	for order := range t.PendingOrders {
		if _, ok := order.Security.(*options.Option); ok {
			return true
		}
	}
	return false
}

func (t *Trader) countOptionLegs() (shortCalls, shortPuts, longCalls, longPuts int) {
	for security, holding := range t.Holdings.Positions {
		opt, ok := security.(*options.Option)
		if !ok {
			continue
		}
		qty := holding.Quantity.Int()
		if opt.Class == 'C' {
			if qty > 0 {
				longCalls += qty
			} else {
				shortCalls += -qty
			}
		} else {
			if qty > 0 {
				longPuts += qty
			} else {
				shortPuts += -qty
			}
		}
	}
	return
}

func (t *Trader) isIVFavorable(o *options.Option) bool {
	metrics := t.Metrics[o]
	if metrics == nil {
		return false
	}
	minIV := metrics.MinIV.Value
	maxIV := metrics.MaxIV.Value
	if minIV.Cmp(maxIV) >= 0 {
		return false
	}
	midIV := minIV.Add(maxIV).Half()
	if t.Config.Direction.IsNegative() {
		// selling: want IV on the high side
		return o.IV.Cmp(midIV) > 0
	}
	// buying: want IV on the low side
	return o.IV.Cmp(midIV) < 0
}

func (t *Trader) hasOrder() (hasBuy bool, hasSell bool) {
	for order := range t.PendingOrders {
		if order.Quantity.IsPositive() {
			hasBuy = true
		} else {
			hasSell = true
		}
	}
	return hasBuy, hasSell
}

func (t *Trader) hedgeDelta(now clocky.Time) {
	for order := range t.PendingOrders {
		if order.Canceling {
			continue
		}
		if now.After(order.Created.Add(t.Config.Patience)) {
			log.Printf("canceling order #%d after waiting %s\n", order.ID, t.Config.Patience)
			err := order.Cancel()
			if err != nil {
				log.Printf("failed to cancel order #%d: %v\n", order.ID, err)
			}
		} else if now.After(order.NextChase) {
			bid := order.Security.GetBid()
			ask := order.Security.GetAsk()
			if order.Quantity.IsPositive() {
				// buying: chase upward if bid improved
				if bid.Cmp(order.Price) > 0 {
					log.Printf("chasing order #%d buy price %s -> %s (bid:%s ask:%s)\n",
						order.ID, order.Price, bid, bid, ask)
					order.Update(bid)
				}
			} else {
				// selling: chase downward if ask improved
				if ask.Cmp(order.Price) < 0 {
					log.Printf("chasing order #%d sell price %s -> %s (bid:%s ask:%s)\n",
						order.ID, order.Price, ask, bid, ask)
					order.Update(ask)
				}
			}
		}
	}
	if now.Before(t.NextHedge) {
		return
	}
	qty := decimal.Zero
	price := decimal.Zero
	spread := t.Config.Spread
	bid := t.Underlying.GetBid()
	ask := t.Underlying.GetAsk()
	mid := bid.Add(ask).Half()
	hlf := ask.Sub(bid).Half()
	hasBuy, hasSell := t.hasOrder()
	tolerance := t.Config.Tolerance.Mul(t.Config.Quantum)
	holding := t.Holdings.Positions[t.Underlying]
	delta := t.computeDelta()
	if !hasBuy && delta.Cmp(tolerance) < 0 {
		if delta.IsNegative() {
			// we genuinely need to buy in order to hedge
			// therefore we must be willing to buy as much as possible
			qty = delta.Neg().QuantizeAway(t.Config.Quantum)
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
			qty = delta.QuantizeAway(t.Config.Quantum).Neg()
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

func (t *Trader) recordFill(now clocky.Time, order *Order, quantity, price decimal.Decimal) {
	t.Transactions = append(t.Transactions, &Transaction{
		Time:     now,
		Security: order.Security,
		Quantity: quantity,
		Price:    price,
		OrderID:  order.ID,
	})
}

func (t *Trader) onOrderFail(order *Order) {
	t.removePendingOrder(order)
	t.NextHedge = clocky.Now().Add(clocky.Second)
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
	if t.Underlying == nil {
		return
	}
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
	metrics := t.getMetrics(o)
	o.TS = m.Header.TSEvent
	mustRecomputeGreeks := false
	bid := m.Levels[0].BidPx
	if bid != databento.UndefPrice {
		price := decimal.Decimal(bid / 1000)
		if price.Cmp(o.Bid) != 0 {
			metrics.BidMA.Add(price)
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
			metrics.AskMA.Add(price)
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
		if o.HasGreeks() {
			metrics.MinIV.Add(o.TS, o.IV)
			metrics.MaxIV.Add(o.TS, o.IV)
		}
	}
}

func (t *Trader) getMetrics(security options.Security) *Metrics {
	metrics := t.Metrics[security]
	if metrics == nil {
		metrics = &Metrics{
			MinIV: indicators.NewMin(t.Config.Lookback),
			MaxIV: indicators.NewMax(t.Config.Lookback),
			BidMA: indicators.NewWWMA(t.Config.Samples),
			AskMA: indicators.NewWWMA(t.Config.Samples),
		}
		t.Metrics[security] = metrics
	}
	return metrics
}
