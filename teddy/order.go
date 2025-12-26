package teddy

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
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
	closeOnce     sync.Once // ensures onClose is only closed once
}

func (o *Order) Wait() {
	<-o.onClose
}

func (o *Order) Cancel() error {
	if o.OrderID == "" || o.State.IsFinal() {
		return ds.ErrOrderNotFound
	}
	if Live {
		switch o.Pair.Exchange.Exchange {
		case ds.ExchangeCoinbase:
			if err := CoinbaseClient.CancelOrder(o.OrderID); err != nil {
				return err
			}
		default:
			panic("unsupported exchange")
		}
	}
	o.kill(ds.OrderStateCanceled)
	return nil
}

// kill transitions order to final state.
func (order *Order) kill(state ds.OrderState) {
	pair := order.Pair
	orders := pair.Exchange.Orders
	order.Lock.Lock()
	order.State = state
	hold := order.Hold
	spent := order.FillValue.Add(order.Fee) // amount of hold consumed by fills
	order.Hold = decimal.Zero
	order.Lock.Unlock()
	switch order.Side {
	case ds.SideBuy:
		// only release unused portion of hold (hold minus what was spent on fills)
		releaseAmount := hold.Sub(spent)
		if releaseAmount.IsPositive() {
			qh := pair.Exchange.Holdings.Get(pair.QuoteCurrency)
			qh.Lock.Lock()
			qh.Available = qh.Available.Add(releaseAmount)
			qh.Check()
			qh.Lock.Unlock()
		}
	case ds.SideSell:
		// for sell orders, hold is in base currency
		// only release unfilled portion (fills already deducted from Quantity)
		order.Lock.RLock()
		filled := order.Filled
		order.Lock.RUnlock()
		releaseAmount := hold.Sub(filled)
		if releaseAmount.IsPositive() {
			bh := pair.Exchange.Holdings.Get(pair.BaseCurrency)
			bh.Lock.Lock()
			bh.Available = bh.Available.Add(releaseAmount)
			bh.Check()
			bh.Lock.Unlock()
		}
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
	order.closeOnce.Do(func() {
		close(order.onClose)
	})
	orders.OnOrderEvent(order)
}

// fill accounts for a fill on an order, computing fee from exchange taker rate.
// Returns the actual filled quantity (may be less than requested if order nearly full)
// and an error if the order cannot accept any more fills.
func (order *Order) fill(filled, value, feeRate decimal.Decimal, force bool) (decimal.Decimal, error) {
	pair := order.Pair
	exchange := pair.Exchange
	orders := exchange.Orders
	holdings := exchange.Holdings
	now := clocky.Now()

	// perform sanity checks
	if !value.IsPositive() {
		panic("fill value must be positive")
	}
	if !filled.IsPositive() {
		panic("fill quantity must be positive")
	}

	// update order atomically, clamping fill to available quantity
	order.Lock.Lock()
	if !force && order.State.IsFinal() {
		order.Lock.Unlock()
		return decimal.Zero, ds.ErrOrderNotFound
	}
	remaining := order.Quantity.Sub(order.Filled)
	if !remaining.IsPositive() {
		order.Lock.Unlock()
		return decimal.Zero, ds.ErrOrderNotFound
	}
	// clamp fill to remaining quantity
	actualFilled := filled.Min(remaining)
	actualValue := value.Mul(actualFilled).Div(filled) // proportional value
	fee := actualValue.Mul(feeRate)
	dir := decimal.Decimal(order.Side)
	total := actualValue.Add(fee.Mul(dir))
	rebate := fee.Mul(exchange.Rebate)

	// detect if kill() already ran and released the hold
	// this happens when a fill update arrives after we locally cancelled
	// in this case, we need to also adjust Available since kill() released it
	holdAlreadyReleased := force && order.Hold.IsZero() && order.State.IsFinal()

	releaseHold := decimal.Zero
	totalFilled := order.Filled.Add(actualFilled)
	isFullyFilled := totalFilled.Cmp(order.Quantity) == 0
	order.LastFillTime = now
	order.Filled = totalFilled
	order.FillValue = order.FillValue.Add(actualValue)
	order.FillPrice = order.FillValue.Div(order.Filled)
	order.Fee = order.Fee.Add(fee)
	if isFullyFilled {
		releaseHold = order.Hold
		order.State = ds.OrderStateFilled
		order.Hold = decimal.Zero
	} else {
		if !force {
			order.State = ds.OrderStatePartiallyFilled
		}
	}
	order.Lock.Unlock()

	switch order.Side {
	case ds.SideBuy:

		// debit cash holding of purchase price and commission
		// additionally we release unused hold if order is fully filled
		quoteHolding := holdings.Get(pair.QuoteCurrency)
		quoteHolding.Lock.Lock()
		quoteHolding.Quantity = quoteHolding.Quantity.Sub(total)
		quoteHolding.Volume = quoteHolding.Volume.Add(actualValue)
		quoteHolding.SellVolume = quoteHolding.SellVolume.Add(actualValue)
		// on full fill, adjust Available for difference between hold and actual cost
		// unusedHold can be negative if Coinbase charged more than we estimated
		if releaseHold.IsPositive() {
			order.Lock.RLock()
			cumulativeSpent := order.FillValue.Add(order.Fee)
			order.Lock.RUnlock()
			unusedHold := releaseHold.Sub(cumulativeSpent)
			quoteHolding.Available = quoteHolding.Available.Add(unusedHold)
		} else if holdAlreadyReleased {
			// kill() already released the hold back to Available, but this fill
			// actually happened so we need to debit Available by the fill cost
			quoteHolding.Available = quoteHolding.Available.Sub(total)
		}
		quoteHolding.Check()
		quoteHolding.Lock.Unlock()

		// credit asset holding with filled quantity
		baseHolding := holdings.Get(pair.BaseCurrency)
		baseHolding.Lock.Lock()
		baseHolding.Available = baseHolding.Available.Add(actualFilled)
		baseHolding.Quantity = baseHolding.Quantity.Add(actualFilled)
		baseHolding.Volume = baseHolding.Volume.Add(actualFilled)
		baseHolding.BuyVolume = baseHolding.BuyVolume.Add(actualFilled)
		baseHolding.Lots.Add(now, actualFilled, total)
		baseHolding.Check()
		baseHolding.Lock.Unlock()

	case ds.SideSell:

		// credit cash holding with sale proceeds minus commission
		quoteHolding := holdings.Get(pair.QuoteCurrency)
		quoteHolding.Lock.Lock()
		quoteHolding.Quantity = quoteHolding.Quantity.Add(total)
		quoteHolding.Available = quoteHolding.Available.Add(total)
		quoteHolding.Volume = quoteHolding.Volume.Add(actualValue)
		quoteHolding.SellVolume = quoteHolding.SellVolume.Add(actualValue)
		quoteHolding.Check()
		quoteHolding.Lock.Unlock()

		// debit asset holding with filled quantity
		// note: Available was already reduced by hold when order was placed
		// the fill consumes from the hold, so we only debit Quantity here
		baseHolding := holdings.Get(pair.BaseCurrency)
		baseHolding.Lock.Lock()
		baseHolding.Quantity = baseHolding.Quantity.Sub(actualFilled)
		if holdAlreadyReleased {
			// kill() already released the hold back to Available, but this fill
			// actually happened so we need to debit Available by the filled amount
			baseHolding.Available = baseHolding.Available.Sub(actualFilled)
		}
		baseHolding.Volume = baseHolding.Volume.Add(actualFilled)
		baseHolding.SellVolume = baseHolding.SellVolume.Add(actualFilled)
		baseHolding.Lots.Consume(actualFilled, decimal.Zero)
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
		gReport.trades[order.Side]++
		gReport.lock.Unlock()
	}

	// notify subscribers that order changed
	orders.OnOrderEvent(order)
	if isFullyFilled {
		order.closeOnce.Do(func() {
			close(order.onClose)
		})
	}

	return actualFilled, nil
}
