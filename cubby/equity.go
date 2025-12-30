package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"log"
	"sync"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

// Equity represents a tradeable stock (e.g., AAPL, SPY).
type Equity struct {
	Symbol     string
	Lock       sync.RWMutex
	Exchange   *Exchange
	LastPrice  decimal.Decimal
	Shares     *Holding // the stock holding (e.g., AAPL shares)
	Trades     map[ds.Side]int
	OnCandle   func(*indicators.Candle)
	OnReady    func()
	repr       string
	isReady    bool
	openOrders *treeset.Set[*Order]
}

func newEquity(exchange *Exchange, symbol string) *Equity {
	if gRunning {
		loggy.Fatalf("cannot create new equity %s while cubby is running", symbol)
	}
	return &Equity{
		Symbol:     symbol,
		Exchange:   exchange,
		Shares:     exchange.Holdings.Get(symbol),
		Trades:     make(map[ds.Side]int),
		OnCandle:   func(*indicators.Candle) {},
		OnReady:    func() {},
		openOrders: treeset.NewWith(compareOrdersByClientOrderID),
		repr:       symbol,
	}
}

func (e *Equity) String() string {
	return e.repr
}

func (e *Equity) run() {
	// Live data daemon would go here
	// For now, only backtest is supported
}

// handleCandle processes a candle update and simulates order fills.
func (e *Equity) handleCandle(candle *indicators.Candle) {
	if gRateLimiter != nil {
		gRateLimiter.Pulse(candle.Start)
	}

	// Update last price from candle close
	e.LastPrice.Store(candle.Close)

	// Mark equity as ready on first candle
	if !e.isReady {
		if *flagVerbose {
			log.Printf("[cubby] %s ready", e)
		}
		e.isReady = true
		e.OnReady()
		e.Exchange.Equities.markReady(e)
	}

	// Simulate order fills based on candle OHLC
	if Paper && e.isReady {
		e.simulateFills(candle)
	}
}

// simulateFills checks open orders and fills them if candle price satisfies limit.
func (e *Equity) simulateFills(candle *indicators.Candle) {
	e.Lock.RLock()
	var orders []*Order
	for i := e.openOrders.Iterator(); i.Next(); {
		orders = append(orders, i.Value())
	}
	e.Lock.RUnlock()

	isOpenCandle := IsMarketOpenCandle(candle.Start)
	isCloseCandle := IsMarketCloseCandle(candle.Start)

	for _, order := range orders {
		if order.State.IsFinal() {
			continue
		}

		var shouldFill bool
		var fillPrice decimal.Decimal
		var isMarketOrder bool

		switch order.Type {
		case ds.OrderTypeMarket:
			// Market orders fill at the open price
			shouldFill = true
			fillPrice = candle.Open
			isMarketOrder = true

		case ds.OrderTypeLimit:
			// Limit orders fill if price touches the limit
			switch order.Side {
			case ds.SideBuy:
				// Buy limit fills if candle Low <= limit price
				if candle.Low.Cmp(order.LimitPrice) <= 0 {
					shouldFill = true
					fillPrice = order.LimitPrice
				}
			case ds.SideSell:
				// Sell limit fills if candle High >= limit price
				if candle.High.Cmp(order.LimitPrice) >= 0 {
					shouldFill = true
					fillPrice = order.LimitPrice
				}
			}

		case ds.OrderTypeMOO:
			// Market-On-Open: fills at open price on market open candle
			if isOpenCandle {
				shouldFill = true
				fillPrice = candle.Open
				isMarketOrder = true
			}

		case ds.OrderTypeMOC:
			// Market-On-Close: fills at close price on market close candle
			if isCloseCandle {
				shouldFill = true
				fillPrice = candle.Close
				isMarketOrder = true
			}

		case ds.OrderTypeLOO:
			// Limit-On-Open: limit order that only fills at market open
			if isOpenCandle {
				switch order.Side {
				case ds.SideBuy:
					// Buy LOO fills if open price <= limit price
					if candle.Open.Cmp(order.LimitPrice) <= 0 {
						shouldFill = true
						fillPrice = candle.Open // fill at open, not limit
					}
				case ds.SideSell:
					// Sell LOO fills if open price >= limit price
					if candle.Open.Cmp(order.LimitPrice) >= 0 {
						shouldFill = true
						fillPrice = candle.Open
					}
				}
			}

		case ds.OrderTypeLOC:
			// Limit-On-Close: limit order that only fills at market close
			if isCloseCandle {
				switch order.Side {
				case ds.SideBuy:
					// Buy LOC fills if close price <= limit price
					if candle.Close.Cmp(order.LimitPrice) <= 0 {
						shouldFill = true
						fillPrice = candle.Close // fill at close, not limit
					}
				case ds.SideSell:
					// Sell LOC fills if close price >= limit price
					if candle.Close.Cmp(order.LimitPrice) >= 0 {
						shouldFill = true
						fillPrice = candle.Close
					}
				}
			}
		}

		if shouldFill {
			order.Lock.RLock()
			unfilled := order.Quantity.Sub(order.Filled)
			order.Lock.RUnlock()

			if unfilled.IsPositive() {
				fillNotional := fillPrice.Mul(unfilled)
				fee := e.Exchange.FeeCalculator.Calculate(clocky.Now(), unfilled.Int(), isMarketOrder)
				_, err := order.fill(unfilled, fillNotional, fee, false)
				if err != nil && *flagVerbose {
					log.Printf("[cubby] fill error for %s: %v", order.ClientOrderID, err)
				}
			}
		}
	}
}

// MarketOrder places a market order (fills at next candle open).
func (e *Equity) MarketOrder(side ds.Side, quantity int) *Order {
	if quantity <= 0 {
		loggy.Fatalf("quantity must be positive")
	}
	qty := decimal.FromInt(quantity)

	order := &Order{
		Equity:        e,
		ClientOrderID: GenerateOrderID(),
		Side:          side,
		Type:          ds.OrderTypeMarket,
		State:         ds.OrderStateNew,
		Quantity:      qty,
		PlacedTime:    e.Exchange.Now(),
		onClose:       make(chan struct{}),
	}

	// Calculate and reserve hold based on current price estimate
	switch side {
	case ds.SideBuy:
		// Reserve buying power - estimate with current price + 5% buffer
		estimatedCost := e.LastPrice.Load().Mul(decimal.Parse("1.05")).Mul(qty)
		e.Exchange.Lock.Lock()
		buyingPower := e.Exchange.DayTradingBuyingPower.Load()
		if buyingPower.Cmp(estimatedCost) < 0 {
			e.Exchange.Lock.Unlock()
			loggy.Fatalf("insufficient buying power: have %s, need %s",
				buyingPower, estimatedCost)
		}
		// Reserve the buying power
		sub(&e.Exchange.DayTradingBuyingPower, estimatedCost)
		e.Exchange.Lock.Unlock()
		order.Hold.Store(estimatedCost)

	case ds.SideSell:
		e.Shares.Lock.Lock()
		if e.Shares.Available.Load().Cmp(qty) < 0 {
			e.Shares.Lock.Unlock()
			loggy.Fatalf("insufficient %s shares: have %s, need %s",
				e.Symbol, e.Shares.Available.Load(), qty)
		}
		sub(&e.Shares.Available, qty)
		e.Shares.Check()
		e.Shares.Lock.Unlock()
		order.Hold.Store(qty)
	}

	// Register order
	e.Exchange.Orders.lock.Lock()
	e.Exchange.Orders.Add(order)
	e.Exchange.Orders.lock.Unlock()

	e.Lock.Lock()
	e.openOrders.Add(order)
	e.Lock.Unlock()

	if *flagVerbose {
		log.Printf("[cubby] placed %s market order for %d %s",
			side, quantity, e.Symbol)
	}

	return order
}

// LimitOrder places a limit order with the given quantity (whole shares) and price.
func (e *Equity) LimitOrder(side ds.Side, quantity int, limitPrice decimal.Decimal, strategy ds.OrderStrategy) *Order {
	if quantity <= 0 {
		loggy.Fatalf("quantity must be positive")
	}
	qty := decimal.FromInt(quantity)

	order := &Order{
		Equity:        e,
		ClientOrderID: GenerateOrderID(),
		Side:          side,
		Type:          ds.OrderTypeLimit,
		State:         ds.OrderStateNew,
		LimitPrice:    limitPrice,
		Quantity:      qty,
		PlacedTime:    e.Exchange.Now(),
		onClose:       make(chan struct{}),
	}

	// Calculate and reserve hold
	switch side {
	case ds.SideBuy:
		// Reserve buying power for purchase
		estimatedCost := limitPrice.Mul(qty)
		e.Exchange.Lock.Lock()
		buyingPower := e.Exchange.DayTradingBuyingPower.Load()
		if buyingPower.Cmp(estimatedCost) < 0 {
			e.Exchange.Lock.Unlock()
			loggy.Fatalf("insufficient buying power: have %s, need %s",
				buyingPower, estimatedCost)
		}
		sub(&e.Exchange.DayTradingBuyingPower, estimatedCost)
		e.Exchange.Lock.Unlock()
		order.Hold.Store(estimatedCost)

	case ds.SideSell:
		// Reserve shares for sale
		e.Shares.Lock.Lock()
		if e.Shares.Available.Load().Cmp(qty) < 0 {
			e.Shares.Lock.Unlock()
			loggy.Fatalf("insufficient %s shares: have %s, need %s",
				e.Symbol, e.Shares.Available.Load(), qty)
		}
		sub(&e.Shares.Available, qty)
		e.Shares.Check()
		e.Shares.Lock.Unlock()
		order.Hold.Store(qty)
	}

	// Register order
	e.Exchange.Orders.lock.Lock()
	e.Exchange.Orders.Add(order)
	e.Exchange.Orders.lock.Unlock()

	e.Lock.Lock()
	e.openOrders.Add(order)
	e.Lock.Unlock()

	if !Paper {
		// Submit to Alpaca
		// TODO: implement live order submission
		loggy.Fatalf("live trading not yet implemented")
	}

	if *flagVerbose {
		log.Printf("[cubby] placed %s limit order for %d %s @ $%s",
			side, quantity, e.Symbol, limitPrice)
	}

	return order
}

// MOOOrder places a Market-On-Open order (fills at next market open).
func (e *Equity) MOOOrder(side ds.Side, quantity int) *Order {
	if quantity <= 0 {
		loggy.Fatalf("quantity must be positive")
	}
	qty := decimal.FromInt(quantity)

	order := &Order{
		Equity:        e,
		ClientOrderID: GenerateOrderID(),
		Side:          side,
		Type:          ds.OrderTypeMOO,
		State:         ds.OrderStateNew,
		Quantity:      qty,
		PlacedTime:    e.Exchange.Now(),
		onClose:       make(chan struct{}),
	}

	switch side {
	case ds.SideBuy:
		// Reserve buying power - estimate with current price + 5% buffer
		estimatedCost := e.LastPrice.Load().Mul(decimal.Parse("1.05")).Mul(qty)
		e.Exchange.Lock.Lock()
		buyingPower := e.Exchange.DayTradingBuyingPower.Load()
		if buyingPower.Cmp(estimatedCost) < 0 {
			e.Exchange.Lock.Unlock()
			loggy.Fatalf("insufficient buying power: have %s, need %s",
				buyingPower, estimatedCost)
		}
		sub(&e.Exchange.DayTradingBuyingPower, estimatedCost)
		e.Exchange.Lock.Unlock()
		order.Hold.Store(estimatedCost)

	case ds.SideSell:
		e.Shares.Lock.Lock()
		if e.Shares.Available.Load().Cmp(qty) < 0 {
			e.Shares.Lock.Unlock()
			loggy.Fatalf("insufficient %s shares: have %s, need %s",
				e.Symbol, e.Shares.Available.Load(), qty)
		}
		sub(&e.Shares.Available, qty)
		e.Shares.Check()
		e.Shares.Lock.Unlock()
		order.Hold.Store(qty)
	}

	e.Exchange.Orders.lock.Lock()
	e.Exchange.Orders.Add(order)
	e.Exchange.Orders.lock.Unlock()

	e.Lock.Lock()
	e.openOrders.Add(order)
	e.Lock.Unlock()

	if *flagVerbose {
		log.Printf("[cubby] placed %s MOO order for %d %s",
			side, quantity, e.Symbol)
	}

	return order
}

// MOCOrder places a Market-On-Close order (fills at market close price).
func (e *Equity) MOCOrder(side ds.Side, quantity int) *Order {
	if quantity <= 0 {
		loggy.Fatalf("quantity must be positive")
	}
	qty := decimal.FromInt(quantity)

	order := &Order{
		Equity:        e,
		ClientOrderID: GenerateOrderID(),
		Side:          side,
		Type:          ds.OrderTypeMOC,
		State:         ds.OrderStateNew,
		Quantity:      qty,
		PlacedTime:    e.Exchange.Now(),
		onClose:       make(chan struct{}),
	}

	switch side {
	case ds.SideBuy:
		estimatedCost := e.LastPrice.Load().Mul(decimal.Parse("1.05")).Mul(qty)
		e.Exchange.Lock.Lock()
		buyingPower := e.Exchange.DayTradingBuyingPower.Load()
		if buyingPower.Cmp(estimatedCost) < 0 {
			e.Exchange.Lock.Unlock()
			loggy.Fatalf("insufficient buying power: have %s, need %s",
				buyingPower, estimatedCost)
		}
		sub(&e.Exchange.DayTradingBuyingPower, estimatedCost)
		e.Exchange.Lock.Unlock()
		order.Hold.Store(estimatedCost)

	case ds.SideSell:
		e.Shares.Lock.Lock()
		if e.Shares.Available.Load().Cmp(qty) < 0 {
			e.Shares.Lock.Unlock()
			loggy.Fatalf("insufficient %s shares: have %s, need %s",
				e.Symbol, e.Shares.Available.Load(), qty)
		}
		sub(&e.Shares.Available, qty)
		e.Shares.Check()
		e.Shares.Lock.Unlock()
		order.Hold.Store(qty)
	}

	e.Exchange.Orders.lock.Lock()
	e.Exchange.Orders.Add(order)
	e.Exchange.Orders.lock.Unlock()

	e.Lock.Lock()
	e.openOrders.Add(order)
	e.Lock.Unlock()

	if *flagVerbose {
		log.Printf("[cubby] placed %s MOC order for %d %s",
			side, quantity, e.Symbol)
	}

	return order
}

// LOOOrder places a Limit-On-Open order (fills at market open if limit is satisfied).
func (e *Equity) LOOOrder(side ds.Side, quantity int, limitPrice decimal.Decimal) *Order {
	if quantity <= 0 {
		loggy.Fatalf("quantity must be positive")
	}
	qty := decimal.FromInt(quantity)

	order := &Order{
		Equity:        e,
		ClientOrderID: GenerateOrderID(),
		Side:          side,
		Type:          ds.OrderTypeLOO,
		State:         ds.OrderStateNew,
		LimitPrice:    limitPrice,
		Quantity:      qty,
		PlacedTime:    e.Exchange.Now(),
		onClose:       make(chan struct{}),
	}

	switch side {
	case ds.SideBuy:
		estimatedCost := limitPrice.Mul(qty)
		e.Exchange.Lock.Lock()
		buyingPower := e.Exchange.DayTradingBuyingPower.Load()
		if buyingPower.Cmp(estimatedCost) < 0 {
			e.Exchange.Lock.Unlock()
			loggy.Fatalf("insufficient buying power: have %s, need %s",
				buyingPower, estimatedCost)
		}
		sub(&e.Exchange.DayTradingBuyingPower, estimatedCost)
		e.Exchange.Lock.Unlock()
		order.Hold.Store(estimatedCost)

	case ds.SideSell:
		e.Shares.Lock.Lock()
		if e.Shares.Available.Load().Cmp(qty) < 0 {
			e.Shares.Lock.Unlock()
			loggy.Fatalf("insufficient %s shares: have %s, need %s",
				e.Symbol, e.Shares.Available.Load(), qty)
		}
		sub(&e.Shares.Available, qty)
		e.Shares.Check()
		e.Shares.Lock.Unlock()
		order.Hold.Store(qty)
	}

	e.Exchange.Orders.lock.Lock()
	e.Exchange.Orders.Add(order)
	e.Exchange.Orders.lock.Unlock()

	e.Lock.Lock()
	e.openOrders.Add(order)
	e.Lock.Unlock()

	if *flagVerbose {
		log.Printf("[cubby] placed %s LOO order for %d %s @ $%s",
			side, quantity, e.Symbol, limitPrice)
	}

	return order
}

// LOCOrder places a Limit-On-Close order (fills at market close if limit is satisfied).
func (e *Equity) LOCOrder(side ds.Side, quantity int, limitPrice decimal.Decimal) *Order {
	if quantity <= 0 {
		loggy.Fatalf("quantity must be positive")
	}
	qty := decimal.FromInt(quantity)

	order := &Order{
		Equity:        e,
		ClientOrderID: GenerateOrderID(),
		Side:          side,
		Type:          ds.OrderTypeLOC,
		State:         ds.OrderStateNew,
		LimitPrice:    limitPrice,
		Quantity:      qty,
		PlacedTime:    e.Exchange.Now(),
		onClose:       make(chan struct{}),
	}

	switch side {
	case ds.SideBuy:
		estimatedCost := limitPrice.Mul(qty)
		e.Exchange.Lock.Lock()
		buyingPower := e.Exchange.DayTradingBuyingPower.Load()
		if buyingPower.Cmp(estimatedCost) < 0 {
			e.Exchange.Lock.Unlock()
			loggy.Fatalf("insufficient buying power: have %s, need %s",
				buyingPower, estimatedCost)
		}
		sub(&e.Exchange.DayTradingBuyingPower, estimatedCost)
		e.Exchange.Lock.Unlock()
		order.Hold.Store(estimatedCost)

	case ds.SideSell:
		e.Shares.Lock.Lock()
		if e.Shares.Available.Load().Cmp(qty) < 0 {
			e.Shares.Lock.Unlock()
			loggy.Fatalf("insufficient %s shares: have %s, need %s",
				e.Symbol, e.Shares.Available.Load(), qty)
		}
		sub(&e.Shares.Available, qty)
		e.Shares.Check()
		e.Shares.Lock.Unlock()
		order.Hold.Store(qty)
	}

	e.Exchange.Orders.lock.Lock()
	e.Exchange.Orders.Add(order)
	e.Exchange.Orders.lock.Unlock()

	e.Lock.Lock()
	e.openOrders.Add(order)
	e.Lock.Unlock()

	if *flagVerbose {
		log.Printf("[cubby] placed %s LOC order for %d %s @ $%s",
			side, quantity, e.Symbol, limitPrice)
	}

	return order
}

// ShortOrder places a short sale market order (sell shares you don't own).
// The shares are borrowed and sold at market. Profit is made if price goes down.
func (e *Equity) ShortOrder(quantity int) *Order {
	if quantity <= 0 {
		loggy.Fatalf("quantity must be positive")
	}

	// Check if symbol is shortable
	if !IsShortable(e.Symbol) {
		loggy.Fatalf("%s is not shortable", e.Symbol)
	}

	qty := decimal.FromInt(quantity)

	order := &Order{
		Equity:        e,
		ClientOrderID: GenerateOrderID(),
		Side:          ds.SideSell,
		Type:          ds.OrderTypeMarket,
		State:         ds.OrderStateNew,
		Quantity:      qty,
		PlacedTime:    e.Exchange.Now(),
		onClose:       make(chan struct{}),
		IsShortSale:   true,
	}

	// Reserve margin for short sale (not buying power)
	// Initial margin is typically 50% for shorts
	marginRequired := InitialMargin(e.Symbol, qty.Neg(), e.LastPrice.Load())

	e.Exchange.Lock.Lock()
	buyingPower := e.Exchange.DayTradingBuyingPower.Load()
	if buyingPower.Cmp(marginRequired) < 0 {
		e.Exchange.Lock.Unlock()
		loggy.Fatalf("insufficient margin for short: have %s, need %s",
			buyingPower, marginRequired)
	}
	sub(&e.Exchange.DayTradingBuyingPower, marginRequired)
	e.Exchange.Lock.Unlock()
	order.Hold.Store(marginRequired)

	// Track available for cover
	e.Shares.Lock.Lock()
	add(&e.Shares.Available, qty) // Available to cover
	e.Shares.Lock.Unlock()

	// Register order
	e.Exchange.Orders.lock.Lock()
	e.Exchange.Orders.Add(order)
	e.Exchange.Orders.lock.Unlock()

	e.Lock.Lock()
	e.openOrders.Add(order)
	e.Lock.Unlock()

	if *flagVerbose {
		log.Printf("[cubby] placed short sale order for %d %s (margin: $%s)",
			quantity, e.Symbol, marginRequired)
	}

	return order
}

// CoverOrder places a buy-to-cover market order to close a short position.
func (e *Equity) CoverOrder(quantity int) *Order {
	if quantity <= 0 {
		loggy.Fatalf("quantity must be positive")
	}
	qty := decimal.FromInt(quantity)

	// Verify we have a short position to cover
	e.Shares.Lock.Lock()
	if !e.Shares.Quantity.Load().IsNegative() {
		e.Shares.Lock.Unlock()
		loggy.Fatalf("cannot cover: %s is not short (quantity: %s)",
			e.Symbol, e.Shares.Quantity.Load())
	}
	absShort := e.Shares.Quantity.Load().Neg()
	available := e.Shares.Available.Load()
	if available.Cmp(qty) < 0 {
		e.Shares.Lock.Unlock()
		loggy.Fatalf("cannot cover %s shares: only %s available (short position: %s)",
			qty, available, absShort)
	}
	// Reserve shares for cover
	sub(&e.Shares.Available, qty)
	e.Shares.Lock.Unlock()

	order := &Order{
		Equity:        e,
		ClientOrderID: GenerateOrderID(),
		Side:          ds.SideBuy,
		Type:          ds.OrderTypeMarket,
		State:         ds.OrderStateNew,
		Quantity:      qty,
		PlacedTime:    e.Exchange.Now(),
		onClose:       make(chan struct{}),
		IsCover:       true,
	}

	// Reserve buying power to cover
	estimatedCost := e.LastPrice.Load().Mul(decimal.Parse("1.05")).Mul(qty)
	e.Exchange.Lock.Lock()
	buyingPower := e.Exchange.DayTradingBuyingPower.Load()
	if buyingPower.Cmp(estimatedCost) < 0 {
		e.Exchange.Lock.Unlock()
		// Restore available
		e.Shares.Lock.Lock()
		add(&e.Shares.Available, qty)
		e.Shares.Lock.Unlock()
		loggy.Fatalf("insufficient buying power to cover: have %s, need %s",
			buyingPower, estimatedCost)
	}
	sub(&e.Exchange.DayTradingBuyingPower, estimatedCost)
	e.Exchange.Lock.Unlock()
	order.Hold.Store(estimatedCost)

	// Register order
	e.Exchange.Orders.lock.Lock()
	e.Exchange.Orders.Add(order)
	e.Exchange.Orders.lock.Unlock()

	e.Lock.Lock()
	e.openOrders.Add(order)
	e.Lock.Unlock()

	if *flagVerbose {
		log.Printf("[cubby] placed cover order for %d %s",
			quantity, e.Symbol)
	}

	return order
}

func compareEquities(a, b *Equity) int {
	if a.Exchange.Exchange < b.Exchange.Exchange {
		return -1
	}
	if a.Exchange.Exchange > b.Exchange.Exchange {
		return +1
	}
	if a.Symbol < b.Symbol {
		return -1
	}
	if a.Symbol > b.Symbol {
		return +1
	}
	return 0
}
