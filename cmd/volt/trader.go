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
	"log"
)

type Trader struct {
	Symbol                symbol.Symbol
	Config                *Config
	Chain                 *options.Options
	Holdings              Holdings
	OrderEvents           chan *schwab.OrderEvent
	OrderUpdates          chan OrderUpdate
	OptionsByID           map[uint32]*options.Option
	OptionsByOSI          map[string]*options.Option
	OrdersBySchwabID      map[schwab.OrderID]*Order
	PendingOrders         map[*Order]bool
	PendingOrdersByOption map[*options.Option][]*Order
	StrategiesUsed        map[string]int
	NextTradeTime         clocky.Time
	MarketClose           clocky.Time
	StopTime              clocky.Time
	PanicTime             clocky.Time
	Hinter                *loggy.Hinter
	Web                   *Web
	OrderCounter          int
	IdentifierCounter     int
	StopTransitioned      bool
	UnderlyingPrice       decimal.Decimal
	Panicking             bool
	Paused                bool
}

func NewTrader(symbol symbol.Symbol, config *Config) *Trader {
	return &Trader{
		Symbol:                symbol,
		Config:                config,
		Chain:                 options.NewOptions(),
		Holdings:              Holdings{Positions: map[*options.Option]*Holding{}},
		OrderEvents:           make(chan *schwab.OrderEvent, 64),
		OrderUpdates:          make(chan OrderUpdate, 64),
		PendingOrders:         map[*Order]bool{},
		PendingOrdersByOption: map[*options.Option][]*Order{},
		OrdersBySchwabID:      map[schwab.OrderID]*Order{},
		OptionsByID:           map[uint32]*options.Option{},
		OptionsByOSI:          map[string]*options.Option{},
		StrategiesUsed:        map[string]int{},
		Hinter:                loggy.NewHinter(),
		StopTime:              clocky.MaxTime,
		PanicTime:             clocky.MaxTime,
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
	if !t.StopTransitioned && clock >= t.Config.StopTrading {
		t.StopTransitioned = true
		for _, k := range kStrategies {
			t.Config.Strategies[k] = false
		}
		log.Printf("STOPPING TRADING")
	}
	if now < t.NextTradeTime {
		t.Hinter.Hint("not trading: cooldown (%s remaining)", clocky.Duration(t.NextTradeTime-now))
		return
	}
	if len(t.PendingOrders) >= t.Config.MaxPending {
		t.Hinter.Hint("not trading: %d pending orders (max %d)", len(t.PendingOrders), t.Config.MaxPending)
		return
	}
	if t.Paused {
		t.Hinter.Hint("not trading: paused")
		return
	}
	t.trade(now)
}

func (t *Trader) trade(now clocky.Time) {
}

func (t *Trader) submitOrder(strategy string, legs []*Leg) bool {
	price := t.choosePriceForOrder(legs)
	if price.IsZero() {
		return false
	}
	t.OrderCounter++
	order := &Order{
		Trader:   t,
		ID:       t.IdentifierCounter,
		Legs:     legs,
		Strategy: strategy,
		Price:    price,
		Created:  clocky.Now(),
	}
	t.IdentifierCounter++
	t.StrategiesUsed[strategy]++
	t.NextTradeTime = order.Created.Add(t.Config.Cooldown)
	err := order.Send()
	if err != nil {
		log.Printf("#%d failed to send order: %v", order.ID, err)
		return false
	}
	log.Printf("#%d %s: price=%s natural=%s maker=%s",
		order.ID, order.Strategy, order.Price, order.NaturalPrice(), order.MakerPrice())
	for _, leg := range legs {
		if leg.Quantity.IsPositive() {
			log.Printf("  buy  %4s  %s", leg.Quantity, leg.Option)
		} else {
			log.Printf("  sell %4s  %s", leg.Quantity.Abs(), leg.Option)
		}
	}
	return true
}

func (t *Trader) choosePriceForOrder(legs []*Leg) decimal.Decimal {
	price := decimal.Zero
	for _, leg := range legs {
		opt := leg.Option
		mid := opt.MidPrice()
		hlf := opt.Ask.Sub(opt.Bid).DivInt(2)
		if leg.Quantity.IsPositive() {
			price = price.Sub(mid.Add(hlf.Mul(t.Config.Spread)))
		} else {
			price = price.Add(mid.Sub(hlf.Mul(t.Config.Spread)))
		}
	}
	tick, bigTick := getTicks(t.Symbol)
	if price.Abs().Cmp(kThree) >= 0 {
		tick = bigTick
	}
	return price.QuantizeCeil(tick)
}

func (t *Trader) cancelUnfilledOrders(now clocky.Time) {
	for order := range t.PendingOrders {
		if order.Canceling || order.Filled() || (*liveFlag && order.OrderID == 0) {
			continue
		}
		elapsed := now.Sub(order.Created)
		if elapsed >= t.Config.Patience {
			err := order.Cancel()
			if err == nil {
				log.Printf("#%d canceling order id %d after %s", order.ID, order.OrderID, elapsed)
			} else {
				log.Printf("#%d failed to cancel order id %d after %s: %v", order.ID, order.OrderID, elapsed, err)
			}
		}
	}
}

func (t *Trader) onHeartbeat() {
	liq := t.computeLiquidationValue()
	delta, gamma, theta, vega := t.computeGreeks()
	log.Printf("cash:%s liq:%s delta:%s gamma:%s theta:%s vega:%s",
		t.Holdings.Cash, liq.Truncate(),
		delta.Format(2), gamma.Format(2), theta.Format(2), vega.Format(2))
}

func (t *Trader) onEndOfDay() {
	log.Printf("end of day settlement time")
	for _, holding := range t.Holdings.Sorted() {
		intrinsic := holding.Option.IntrinsicValue(t.Chain.Price)
		settlement := intrinsic.Mul(holding.Quantity).MulInt(kMultiplier)
		t.Holdings.Cash = t.Holdings.Cash.Add(settlement)
		log.Printf("have %4s of %s worth %8s", holding.Quantity, holding.Option.StringAligned(), settlement.Truncate())
	}
	log.Printf("strategies used")
	for _, s := range kStrategies {
		count := t.StrategiesUsed[s]
		log.Printf("%30s %4d", s, count)
	}
	totalFees := t.Holdings.TotalFees
	log.Printf("ending balance %s less %s fees winning %s at %s price %s",
		t.Holdings.Cash, totalFees, t.Holdings.Cash.Sub(totalFees), t.Symbol, t.Chain.Price)
}

// iteratePositions calls f for all positions.
func (t *Trader) iteratePositions(f func(*options.Option, decimal.Decimal)) {
	for option, position := range t.Holdings.Positions {
		f(option, position.Quantity)
	}
}

func (t *Trader) computeSettlementAt(underlyingPrice decimal.Decimal) decimal.Decimal {
	value := decimal.Zero
	t.iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		intrinsic := option.IntrinsicValue(underlyingPrice)
		value = value.Add(intrinsic.Mul(pos))
	})
	return value.MulInt(kMultiplier).Add(t.Holdings.Cash)
}

func (t *Trader) computeLiquidationValue() decimal.Decimal {
	value := decimal.Zero
	t.iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		mid := option.MidPrice()
		value = value.Add(mid.Mul(pos))
	})
	return value.MulInt(kMultiplier).Add(t.Holdings.Cash)
}

// computeDelta calculates the delta for all positions.
func (t *Trader) computeDelta() decimal.Decimal {
	delta := decimal.Zero
	t.iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		delta = delta.Add(option.Delta.Mul(pos))
	})
	return delta.MulInt(kMultiplier)
}

func (t *Trader) computeGreeks() (delta, gamma, theta, vega decimal.Decimal) {
	t.iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		delta = delta.Add(option.Delta.Mul(pos))
		gamma = gamma.Add(option.Gamma.Mul(pos))
		theta = theta.Add(option.Theta.Mul(pos))
		vega = vega.Add(option.Vega.Mul(pos))
	})
	delta = delta.MulInt(kMultiplier)
	gamma = gamma.MulInt(kMultiplier)
	theta = theta.MulInt(kMultiplier)
	vega = vega.MulInt(kMultiplier)
	return
}

func (t *Trader) onOptionDef(o *options.Option) {
	osi := o.OSI()
	if existing := t.OptionsByOSI[osi]; existing != nil {
		existing.ID = o.ID
		o = existing
	}
	t.OptionsByID[o.ID] = o
	t.OptionsByOSI[osi] = o
}

func (t *Trader) onOptionDefEnd() {

	// get close time
	now := clocky.Now()
	year, month, day := now.Date()
	t.MarketClose = cboe.GetCloseTime(year, month, day)

	// initialize options restrictions after loading schwab order history
	for option, holding := range t.Holdings.Positions {
		if holding.Quantity.IsPositive() {
			option.Mode = options.ModeLong
		} else if holding.Quantity.IsNegative() {
			option.Mode = options.ModeShort
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
	if t.Chain.AtTheMoney != nil && t.Chain.AtTheMoney.Call.Mode == options.ModeNone {
		t.Chain.AtTheMoney.Call.Mode = options.ModeShort
		t.Chain.AtTheMoney.Put.Mode = options.ModeLong
	}

	// now color remaining strikes as checkered as possible
	if t.Chain.AtTheMoney != nil {
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

func (t *Trader) onUnderlyingTick(m *databento.MBP1) {
	bid := m.Levels[0].BidPx
	ask := m.Levels[0].AskPx
	if bid != databento.UndefPrice && ask != databento.UndefPrice {
		bidPrice := decimal.Decimal(bid / 1000)
		askPrice := decimal.Decimal(ask / 1000)
		if bidPrice.IsPositive() && askPrice.IsPositive() {
			t.UnderlyingPrice = bidPrice.Add(askPrice).DivInt(2)
		}
	}
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
		o.ComputeGreeks(t.Chain.Price, kRiskFreeRate, t.UnderlyingPrice)
	}
	t.arbitrageBox(o.Strike)
}
