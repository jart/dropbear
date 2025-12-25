package teddy

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"sync"
)

type Order struct {
	Lock          sync.RWMutex
	Pair          *Pair
	OrderID       string
	ClientOrderID string
	Side          ds.Side
	Type          ds.OrderType
	State         ds.OrderState
	LimitPrice    decimal.Decimal
	FillValue     decimal.Decimal
	FillPrice     decimal.Decimal
	Quantity      decimal.Decimal // filled + unfilled base size
	Filled        decimal.Decimal // filled base size
	Hold          decimal.Decimal
	Fee           decimal.Decimal
	PlacedTime    clocky.Time     // when we created the object
	CreatedTime   clocky.Time     // when exchange says it was created
	LastFillTime  clocky.Time     // when last fill occurred
	lastFees      decimal.Decimal // used for coinbase fee tracking
	onClose       chan struct{}
}

func (o *Order) Wait() {
	<-o.onClose
}

// kill transitions order to final state.
func (order *Order) kill(state ds.OrderState) {
	pair := order.Pair
	orders := pair.Exchange.Orders
	order.Lock.Lock()
	order.State = state
	hold := order.Hold
	order.Hold = decimal.Zero
	order.Lock.Unlock()
	switch order.Side {
	case ds.SideBuy:
		qh := pair.Exchange.Holdings.Get(pair.QuoteCurrency)
		qh.Lock.Lock()
		qh.Available = qh.Available.Add(hold)
		qh.Check()
		qh.Lock.Unlock()
	case ds.SideSell:
		bh := pair.Exchange.Holdings.Get(pair.BaseCurrency)
		bh.Lock.Lock()
		bh.Available = bh.Available.Add(hold)
		bh.Check()
		bh.Lock.Unlock()
	}
	orders.lock.Lock()
	if orders.openOrders.Contains(order) {
		orders.openOrders.Remove(order)
	}
	orders.lock.Unlock()
	pair.Lock.Lock()
	if pair.openOrders.Contains(order) {
		pair.openOrders.Remove(order)
	}
	pair.Lock.Unlock()
	close(order.onClose)
	orders.OnOrderEvent(order)
}

// fill accounts for a fill on an order, computing fee from exchange taker rate.
// This is the backtest entry point.
func (order *Order) fill(filled, value, feeRate decimal.Decimal) {
	pair := order.Pair
	exchange := pair.Exchange
	orders := exchange.Orders
	holdings := exchange.Holdings
	fee := value.Mul(feeRate)
	now := clocky.Now()
	dir := decimal.Decimal(order.Side)
	total := value.Add(fee.Mul(dir))
	rebate := fee.Mul(exchange.Rebate)

	// perform sanity checks
	if !value.IsPositive() {
		loggy.Fatalf("fill value must be positive")
	}
	if !filled.IsPositive() {
		loggy.Fatalf("fill quantity must be positive")
	}
	if order.State.IsFinal() {
		loggy.Fatalf("cannot fill order %s in final state %s", order.ClientOrderID, order.State)
	}

	// update order
	order.Lock.Lock()
	releaseHold := decimal.Zero
	totalFilled := order.Filled.Add(filled)
	if totalFilled.Cmp(order.Quantity) > 0 {
		loggy.Fatalf("overfill detected on order %s: total filled %s %s exceeds quantity %s %s",
			order.ClientOrderID, totalFilled, pair.BaseCurrency, order.Quantity, pair.BaseCurrency)
	}
	isFullyFilled := totalFilled.Cmp(order.Quantity) == 0
	order.LastFillTime = now
	order.Filled = totalFilled
	order.FillValue = order.FillValue.Add(value)
	order.FillPrice = order.FillValue.Div(order.Filled)
	order.Fee = order.Fee.Add(fee)
	if isFullyFilled {
		releaseHold = order.Hold
		order.State = ds.OrderStateFilled
		order.Hold = decimal.Zero
	} else {
		order.State = ds.OrderStatePartiallyFilled
	}
	order.Lock.Unlock()

	switch order.Side {
	case ds.SideBuy:

		// debit cash holding of purchase price and commission
		// additionally we release unused hold if order is fully filled
		quoteHolding := holdings.Get(pair.QuoteCurrency)
		quoteHolding.Lock.Lock()
		quoteHolding.Quantity = quoteHolding.Quantity.Sub(total)
		quoteHolding.Volume = quoteHolding.Volume.Add(value)
		quoteHolding.SellVolume = quoteHolding.SellVolume.Add(value)
		// on full fill, release unused portion of hold (hold was for slippage buffer)
		// use cumulative totals since fills may span multiple order book levels
		if releaseHold.IsPositive() {
			order.Lock.RLock()
			cumulativeSpent := order.FillValue.Add(order.Fee)
			order.Lock.RUnlock()
			unusedHold := releaseHold.Sub(cumulativeSpent)
			if unusedHold.IsPositive() {
				quoteHolding.Available = quoteHolding.Available.Add(unusedHold)
			}
		}
		quoteHolding.Check()
		quoteHolding.Lock.Unlock()

		// credit asset holding with filled quantity
		baseHolding := holdings.Get(pair.BaseCurrency)
		baseHolding.Lock.Lock()
		baseHolding.Available = baseHolding.Available.Add(filled)
		baseHolding.Quantity = baseHolding.Quantity.Add(filled)
		baseHolding.Volume = baseHolding.Volume.Add(filled)
		baseHolding.BuyVolume = baseHolding.BuyVolume.Add(filled)
		baseHolding.Lots.Add(now, filled, total)
		baseHolding.Check()
		baseHolding.Lock.Unlock()

	case ds.SideSell:

		// credit cash holding with sale proceeds minus commission
		quoteHolding := holdings.Get(pair.QuoteCurrency)
		quoteHolding.Lock.Lock()
		quoteHolding.Quantity = quoteHolding.Quantity.Add(total)
		quoteHolding.Available = quoteHolding.Available.Add(total)
		quoteHolding.Volume = quoteHolding.Volume.Add(value)
		quoteHolding.SellVolume = quoteHolding.SellVolume.Add(value)
		quoteHolding.Check()
		quoteHolding.Lock.Unlock()

		// debit asset holding with filled quantity
		// note: Available was already reduced by hold when order was placed
		// the fill consumes from the hold, so we only debit Quantity here
		baseHolding := holdings.Get(pair.BaseCurrency)
		baseHolding.Lock.Lock()
		baseHolding.Quantity = baseHolding.Quantity.Sub(filled)
		baseHolding.Volume = baseHolding.Volume.Add(filled)
		baseHolding.SellVolume = baseHolding.SellVolume.Add(filled)
		totalConsumed, err := baseHolding.Lots.Consume(filled, now, decimal.Zero)
		if err != nil {
			loggy.Fatalf("could only consume lots for %s %s out of %s %s: %v",
				filled, pair.BaseCurrency, totalConsumed, pair.BaseCurrency, err)
		}
		baseHolding.Check()
		baseHolding.Lock.Unlock()
	}

	// credit usdc holding with commission rebate
	usdcHolding := holdings.Get("USDC")
	usdcHolding.Lock.Lock()
	usdcHolding.Available = usdcHolding.Available.Add(rebate)
	usdcHolding.Quantity = usdcHolding.Quantity.Add(rebate)
	usdcHolding.Check()
	usdcHolding.Lock.Unlock()

	// update orders manager
	if isFullyFilled {
		orders.lock.Lock()
		if orders.openOrders.Contains(order) {
			orders.openOrders.Remove(order)
		}
		orders.lock.Unlock()
		pair.Lock.Lock()
		if pair.openOrders.Contains(order) {
			pair.openOrders.Remove(order)
		}
		pair.Lock.Unlock()
	}

	// track additional metrics
	exchange.Lock.Lock()
	exchange.Fees = exchange.Fees.Add(fee)
	exchange.Lock.Unlock()
	if gReport != nil {
		gReport.lock.Lock()
		gReport.trades++
		gReport.buys++
		gReport.lock.Unlock()
	}

	// notify subscribers that order changed
	orders.OnOrderEvent(order)
	if isFullyFilled {
		close(order.onClose)
	}
}
