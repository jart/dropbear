package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"log"
	"sync"
)

type Order struct {
	Lock          sync.RWMutex
	Equity        *Equity
	OrderID       string
	ClientOrderID string
	Side          ds.Side
	Type          ds.OrderType
	State         ds.OrderState
	LimitPrice    decimal.Decimal
	Quantity      decimal.Decimal // number of shares (whole shares only)
	Filled        decimal.Decimal // filled shares
	Notional      decimal.Decimal // filled USD value
	Price         decimal.Decimal // average fill price
	Hold          decimal.Decimal
	Fee           decimal.Decimal
	PlacedTime    clocky.Time
	CreatedTime   clocky.Time
	LastFillTime  clocky.Time
	onClose       chan struct{}
	closeOnce     sync.Once
}

func (o *Order) Wait() {
	<-o.onClose
}

func (o *Order) Cancel() error {
	if o.OrderID == "" || o.State.Load().IsFinal() {
		return ds.ErrOrderNotFound
	}
	if !Paper {
		if err := AlpacaClient.CancelOrder(o.OrderID); err != nil {
			return err
		}
	}
	o.kill(ds.OrderStateCanceled)
	return nil
}

// kill transitions order to final state.
func (order *Order) kill(state ds.OrderState) {
	eq := order.Equity
	orders := eq.Exchange.Orders
	order.Lock.Lock()
	order.State.Store(state)
	hold := order.Hold.Load()
	spent := order.Notional.Load().Add(order.Fee.Load())
	order.Hold.Store(decimal.Zero)
	order.Lock.Unlock()
	switch order.Side {
	case ds.SideBuy:
		releaseAmount := hold.Sub(spent)
		if releaseAmount.IsPositive() {
			eq.Cash.Lock.Lock()
			add(&eq.Cash.Available, releaseAmount)
			eq.Cash.Check()
			eq.Cash.Lock.Unlock()
		}
	case ds.SideSell:
		order.Lock.RLock()
		filled := order.Filled.Load()
		order.Lock.RUnlock()
		releaseAmount := hold.Sub(filled)
		if releaseAmount.IsPositive() {
			eq.Shares.Lock.Lock()
			add(&eq.Shares.Available, releaseAmount)
			eq.Shares.Check()
			eq.Shares.Lock.Unlock()
		}
	}
	orders.lock.Lock()
	if orders.openOrders.Contains(order) {
		orders.openOrders.Remove(order)
	}
	orders.lock.Unlock()
	func() {
		eq.Lock.Lock()
		defer eq.Lock.Unlock()
		if eq.openOrders.Contains(order) {
			eq.openOrders.Remove(order)
		}
	}()
	order.closeOnce.Do(func() {
		close(order.onClose)
	})
	if fn := orders.OnOrderEvent.Load(); fn != nil {
		(*fn)(order)
	}
}

// fill accounts for a fill on an order.
// feeRate is 0 for Alpaca stocks (commission-free).
func (order *Order) fill(filled, notional, feeRate decimal.Decimal, force bool) (decimal.Decimal, error) {
	eq := order.Equity
	exchange := eq.Exchange
	orders := exchange.Orders
	now := clocky.Now()

	if !notional.IsPositive() {
		panic("fill notional must be positive")
	}
	if !filled.IsPositive() {
		panic("fill quantity must be positive")
	}

	order.Lock.Lock()
	if !force && order.State.Load().IsFinal() {
		order.Lock.Unlock()
		return decimal.Zero, ds.ErrOrderNotFound
	}
	remaining := order.Quantity.Load().Sub(order.Filled.Load())
	if !remaining.IsPositive() {
		order.Lock.Unlock()
		return decimal.Zero, ds.ErrOrderNotFound
	}

	actualFilled := filled.Min(remaining)
	// Divide first to avoid overflow when notional*actualFilled is huge
	actualNotional := notional.Div(filled).Mul(actualFilled)
	fee := actualNotional.Mul(feeRate)
	dir := decimal.Decimal(order.Side)
	total := actualNotional.Add(fee.Mul(dir))

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
		eq.Cash.Lock.Lock()
		eq.Cash.Volume += actualNotional.Float64()
		eq.Cash.SellVolume += actualNotional.Float64()
		sub(&eq.Cash.Quantity, total)
		if releaseHold.IsPositive() {
			order.Lock.RLock()
			cumulativeSpent := order.Notional.Load().Add(order.Fee.Load())
			order.Lock.RUnlock()
			unusedHold := releaseHold.Sub(cumulativeSpent)
			add(&eq.Cash.Available, unusedHold)
		} else if holdAlreadyReleased {
			sub(&eq.Cash.Available, total)
		}
		eq.Cash.Check()
		eq.Cash.Lock.Unlock()

		eq.Shares.Lock.Lock()
		add(&eq.Shares.Available, actualFilled)
		add(&eq.Shares.Quantity, actualFilled)
		eq.Shares.Volume += actualFilled.Float64()
		eq.Shares.BuyVolume += actualFilled.Float64()
		eq.Shares.Lots.Add(now, actualFilled, total)
		eq.Shares.Check()
		eq.Shares.Lock.Unlock()

	case ds.SideSell:
		eq.Cash.Lock.Lock()
		add(&eq.Cash.Quantity, total)
		add(&eq.Cash.Available, total)
		eq.Cash.Volume += actualNotional.Float64()
		eq.Cash.SellVolume += actualNotional.Float64()
		eq.Cash.Check()
		eq.Cash.Lock.Unlock()

		eq.Shares.Lock.Lock()
		sub(&eq.Shares.Quantity, actualFilled)
		if holdAlreadyReleased {
			sub(&eq.Shares.Available, actualFilled)
		}
		eq.Shares.Volume += actualFilled.Float64()
		eq.Shares.SellVolume += actualFilled.Float64()
		eq.Shares.Lots.Consume(actualFilled, decimal.Zero)
		eq.Shares.Check()
		eq.Shares.Lock.Unlock()
	}

	if isFullyFilled {
		orders.lock.Lock()
		if orders.openOrders.Contains(order) {
			orders.openOrders.Remove(order)
		}
		orders.lock.Unlock()
		eq.Lock.Lock()
		if eq.openOrders.Contains(order) {
			eq.openOrders.Remove(order)
		}
		eq.Lock.Unlock()
	}

	exchange.Lock.Lock()
	exchange.Fees = exchange.Fees.Add(fee)
	exchange.Lock.Unlock()
	eq.Lock.Lock()
	eq.Trades[order.Side]++
	eq.Lock.Unlock()

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
		log.Printf("[cubby] %s %s %s%% filled %s @ $%s",
			order.Side, order.Type, percentFilled, filled, order.Price)
	}
	return actualFilled, nil
}
