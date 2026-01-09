package teddy

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"log"
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
	Quantity      decimal.Decimal // filled + unfilled base size
	Filled        decimal.Decimal // filled base size
	Notional      decimal.Decimal // filled quote value
	Price         decimal.Decimal // average fill price
	Hold          decimal.Decimal
	Fee           decimal.Decimal
	PlacedTime    clocky.Time     // when we created the object
	CreatedTime   clocky.Time     // when broker says it was created
	LastFillTime  clocky.Time     // when last fill occurred
	lastFees      decimal.Decimal // used for coinbase fee tracking
	onClose       chan struct{}
	closeOnce     sync.Once // ensures onClose is only closed once
}

func (o *Order) Wait() {
	<-o.onClose
}

func (o *Order) Cancel() error {
	if o.OrderID == "" || o.State.Load().IsFinal() {
		return ds.ErrNotFound
	}
	if !Paper {
		switch o.Pair.Broker.Broker {
		case ds.BrokerCoinbase:
			if err := CoinbaseClient.CancelOrder(o.OrderID); err != nil {
				return err
			}
		default:
			panic("unsupported broker")
		}
	}
	o.kill(ds.OrderStateCanceled)
	return nil
}

// kill transitions order to final state.
func (order *Order) kill(state ds.OrderState) {
	pair := order.Pair
	orders := pair.Broker.Orders
	order.Lock.Lock()
	order.State.Store(state)
	hold := order.Hold.Load()
	spent := order.Notional.Load().Add(order.Fee.Load()) // amount of hold consumed by fills
	order.Hold.Store(decimal.Zero)
	order.Lock.Unlock()
	switch order.Side {
	case ds.SideBuy:
		// only release unused portion of hold (hold minus what was spent on fills)
		releaseAmount := hold.Sub(spent)
		if releaseAmount.IsPositive() {
			pair.QuoteCurrency.Lock.Lock()
			add(&pair.QuoteCurrency.Available, releaseAmount)
			pair.QuoteCurrency.Check()
			pair.QuoteCurrency.Lock.Unlock()
		}
	case ds.SideSell:
		// for sell orders, hold is in base currency
		// only release unfilled portion (fills already deducted from Quantity)
		order.Lock.RLock()
		filled := order.Filled.Load()
		order.Lock.RUnlock()
		releaseAmount := hold.Sub(filled)
		if releaseAmount.IsPositive() {
			pair.BaseCurrency.Lock.Lock()
			add(&pair.BaseCurrency.Available, releaseAmount)
			pair.BaseCurrency.Check()
			pair.BaseCurrency.Lock.Unlock()
		}
	}
	orders.lock.Lock()
	if orders.openOrders.Contains(order) {
		orders.openOrders.Remove(order)
	}
	orders.lock.Unlock()
	func() {
		pair.Lock.Lock()
		defer pair.Lock.Unlock()
		if pair.openOrders.Contains(order) {
			pair.openOrders.Remove(order)
		}
	}()
	order.closeOnce.Do(func() {
		close(order.onClose)
	})
	if fn := orders.OnOrderEvent.Load(); fn != nil {
		(*fn)(order)
	}
}

// fill accounts for a fill on an order, computing fee from broker taker rate.
// Returns the actual filled quantity (may be less than requested if order nearly full)
// and an error if the order cannot accept any more fills.
func (order *Order) fill(filled, notional, feeRate decimal.Decimal, force bool) (decimal.Decimal, error) {
	pair := order.Pair
	broker := pair.Broker
	orders := broker.Orders
	now := clocky.Now()

	// perform sanity checks
	if !notional.IsPositive() {
		panic("fill notional must be positive")
	}
	if !filled.IsPositive() {
		panic("fill quantity must be positive")
	}

	// update order atomically, clamping fill to available quantity
	order.Lock.Lock()
	if !force && order.State.Load().IsFinal() {
		order.Lock.Unlock()
		return decimal.Zero, ds.ErrNotFound
	}
	remaining := order.Quantity.Load().Sub(order.Filled.Load())
	if !remaining.IsPositive() {
		order.Lock.Unlock()
		return decimal.Zero, ds.ErrNotFound
	}

	// clamp fill to remaining quantity
	actualFilled := filled.Min(remaining)
	actualNotional := notional.Mul(actualFilled).Div(filled) // proportional notional
	fee := actualNotional.Mul(feeRate)
	dir := decimal.Decimal(order.Side)
	total := actualNotional.Add(fee.Mul(dir))
	rebate := fee.Mul(broker.Rebate.Load())

	// detect if kill() already ran and released the hold
	// this happens when a fill update arrives after we locally cancelled
	// in this case, we need to also adjust Available since kill() released it
	holdAlreadyReleased := force && order.Hold.Load().IsZero() && order.State.Load().IsFinal()

	releaseHold := decimal.Zero
	totalFilled := order.Filled.Load().Add(actualFilled)
	isFullyFilled := totalFilled.Cmp(order.Quantity.Load()) == 0
	order.LastFillTime.Store(now)
	order.Filled.Store(totalFilled)
	add(&order.Fee, fee)
	add(&order.Notional, actualNotional)
	order.Price.Store(order.Notional.Load().Div(order.Filled.Load()))
	if isFullyFilled {
		releaseHold = order.Hold.Load()
		order.State.Store(ds.OrderStateFilled)
		order.Hold.Store(decimal.Zero)
	} else {
		if !force {
			order.State.Store(ds.OrderStatePartiallyFilled)
		}
	}
	order.Lock.Unlock()

	switch order.Side {
	case ds.SideBuy:

		// debit cash holding of purchase price and commission
		// additionally we release unused hold if order is fully filled
		pair.QuoteCurrency.Lock.Lock()
		pair.QuoteCurrency.Volume += actualNotional.Float64()
		pair.QuoteCurrency.SellVolume += actualNotional.Float64()
		sub(&pair.QuoteCurrency.Quantity, total)
		// on full fill, adjust Available for difference between hold and actual cost
		// unusedHold can be negative if Coinbase charged more than we estimated
		if releaseHold.IsPositive() {
			order.Lock.RLock()
			cumulativeSpent := order.Notional.Load().Add(order.Fee.Load())
			order.Lock.RUnlock()
			unusedHold := releaseHold.Sub(cumulativeSpent)
			add(&pair.QuoteCurrency.Available, unusedHold)
		} else if holdAlreadyReleased {
			// kill() already released the hold back to Available, but this fill
			// actually happened so we need to debit Available by the fill cost
			sub(&pair.QuoteCurrency.Available, total)
		}
		pair.QuoteCurrency.Check()
		pair.QuoteCurrency.Lock.Unlock()

		// credit asset holding with filled quantity
		pair.BaseCurrency.Lock.Lock()
		add(&pair.BaseCurrency.Available, actualFilled)
		add(&pair.BaseCurrency.Quantity, actualFilled)
		pair.BaseCurrency.Volume += actualFilled.Float64()
		pair.BaseCurrency.BuyVolume += actualFilled.Float64()
		pair.BaseCurrency.Lots.Add(now, actualFilled, total)
		pair.BaseCurrency.Check()
		pair.BaseCurrency.Lock.Unlock()

	case ds.SideSell:

		// credit cash holding with sale proceeds minus commission
		pair.QuoteCurrency.Lock.Lock()
		add(&pair.QuoteCurrency.Quantity, total)
		add(&pair.QuoteCurrency.Available, total)
		pair.QuoteCurrency.Volume += actualNotional.Float64()
		pair.QuoteCurrency.SellVolume += actualNotional.Float64()
		pair.QuoteCurrency.Check()
		pair.QuoteCurrency.Lock.Unlock()

		// debit asset holding with filled quantity
		// note: Available was already reduced by hold when order was placed
		// the fill consumes from the hold, so we only debit Quantity here
		pair.BaseCurrency.Lock.Lock()
		sub(&pair.BaseCurrency.Quantity, actualFilled)
		if holdAlreadyReleased {
			// kill() already released the hold back to Available, but this fill
			// actually happened so we need to debit Available by the filled amount
			sub(&pair.BaseCurrency.Available, actualFilled)
		}
		pair.BaseCurrency.Volume += actualFilled.Float64()
		pair.BaseCurrency.SellVolume += actualFilled.Float64()
		pair.BaseCurrency.Lots.Consume(actualFilled, decimal.Zero)
		pair.BaseCurrency.Check()
		pair.BaseCurrency.Lock.Unlock()
	}

	// credit usdc holding with commission rebate
	usdcHolding := broker.Holdings.Get("USDC")
	usdcHolding.Lock.Lock()
	add(&usdcHolding.Quantity, rebate)
	add(&usdcHolding.Available, rebate)
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
	broker.Lock.Lock()
	broker.Fees = broker.Fees.Add(fee)
	broker.Lock.Unlock()
	pair.Lock.Lock()
	pair.Trades[order.Side]++
	pair.Lock.Unlock()

	// notify subscribers that order changed
	if fn := orders.OnOrderEvent.Load(); fn != nil {
		(*fn)(order)
	}
	if isFullyFilled {
		order.closeOnce.Do(func() {
			close(order.onClose)
		})
	}

	if *flagVerbose {
		percentFilled := order.Filled.Div(order.Quantity).MulInt(100).Truncate()
		log.Printf("[teddy] %s limit %s%% filled %s @ $%s",
			order.Side, percentFilled, filled, order.LimitPrice)
	}
	return actualFilled, nil
}
