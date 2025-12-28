package cubby

import (
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
	Cash       *Holding // USD cash holding
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
	e := &Equity{
		Symbol:     symbol,
		Exchange:   exchange,
		Shares:     exchange.Holdings.Get(symbol),
		Cash:       exchange.Holdings.Get("USD"),
		Trades:     make(map[ds.Side]int),
		OnCandle:   func(*indicators.Candle) {},
		OnReady:    func() {},
		openOrders: treeset.NewWith(compareOrdersByClientOrderID),
		repr:       symbol,
	}
	return e
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

	for _, order := range orders {
		if order.State.IsFinal() {
			continue
		}

		var shouldFill bool
		var fillPrice decimal.Decimal

		switch order.Type {
		case ds.OrderTypeMarket:
			// Market orders fill at the open price
			shouldFill = true
			fillPrice = candle.Open

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
		}

		if shouldFill {
			order.Lock.RLock()
			unfilled := order.Quantity.Sub(order.Filled)
			order.Lock.RUnlock()

			if unfilled.IsPositive() {
				fillNotional := fillPrice.Mul(unfilled)
				// Zero fee for Alpaca stocks
				_, err := order.fill(unfilled, fillNotional, decimal.Zero, false)
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
		// Reserve cash - estimate with current price + 5% buffer
		estimatedCost := e.LastPrice.Load().MulInt(105).DivInt(100).Mul(qty)
		e.Cash.Lock.Lock()
		if e.Cash.Available.Cmp(estimatedCost) < 0 {
			e.Cash.Lock.Unlock()
			loggy.Fatalf("insufficient USD balance: have %s, need %s",
				e.Cash.Available, estimatedCost)
		}
		sub(&e.Cash.Available, estimatedCost)
		e.Cash.Check()
		e.Cash.Lock.Unlock()
		order.Hold.Store(estimatedCost)

	case ds.SideSell:
		e.Shares.Lock.Lock()
		if e.Shares.Available.Cmp(qty) < 0 {
			e.Shares.Lock.Unlock()
			loggy.Fatalf("insufficient %s shares: have %s, need %s",
				e.Symbol, e.Shares.Available, qty)
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
		// Reserve cash for purchase
		estimatedCost := limitPrice.Mul(qty)
		e.Cash.Lock.Lock()
		if e.Cash.Available.Cmp(estimatedCost) < 0 {
			e.Cash.Lock.Unlock()
			loggy.Fatalf("insufficient USD balance: have %s, need %s",
				e.Cash.Available, estimatedCost)
		}
		sub(&e.Cash.Available, estimatedCost)
		e.Cash.Check()
		e.Cash.Lock.Unlock()
		order.Hold.Store(estimatedCost)

	case ds.SideSell:
		// Reserve shares for sale
		e.Shares.Lock.Lock()
		if e.Shares.Available.Cmp(qty) < 0 {
			e.Shares.Lock.Unlock()
			loggy.Fatalf("insufficient %s shares: have %s, need %s",
				e.Symbol, e.Shares.Available, qty)
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
