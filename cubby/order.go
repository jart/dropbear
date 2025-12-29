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
	exchange := eq.Exchange
	orders := exchange.Orders
	order.Lock.Lock()
	order.State.Store(state)
	hold := order.Hold.Load()
	spent := order.Notional.Load().Add(order.Fee.Load())
	order.Hold.Store(decimal.Zero)
	order.Lock.Unlock()
	switch order.Side {
	case ds.SideBuy:
		// Release unused buying power back
		releaseAmount := hold.Sub(spent)
		if releaseAmount.IsPositive() {
			exchange.Lock.Lock()
			add(&exchange.DayTradingBuyingPower, releaseAmount)
			exchange.Lock.Unlock()
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
// fee is the absolute fee amount to charge (can be negative for maker rebates).
func (order *Order) fill(filled, notional, fee decimal.Decimal, force bool) (decimal.Decimal, error) {
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
	// Scale fee proportionally if partial fill (divide first to avoid overflow)
	actualFee := fee.Div(filled).Mul(actualFilled)
	dir := decimal.Decimal(order.Side)
	total := actualNotional.Add(actualFee.Mul(dir))

	holdAlreadyReleased := force && order.Hold.Load().IsZero() && order.State.Load().IsFinal()

	releaseHold := decimal.Zero
	totalFilled := order.Filled.Load().Add(actualFilled)
	isFullyFilled := totalFilled.Cmp(order.Quantity.Load()) == 0
	order.LastFillTime.Store(now)
	order.Filled.Store(totalFilled)
	add(&order.Fee, actualFee)
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

	// Log the fill (if verbose)
	if Live || *flagVerbose {
		price := actualNotional.Div(actualFilled)
		log.Printf("%s %s %s @ %s ($%s)",
			order.Side.String(), actualFilled.String(), eq.Symbol,
			price.Format(2), actualNotional.Format(2))
	}

	switch order.Side {
	case ds.SideBuy:
		// Deduct cash from exchange
		exchange.Lock.Lock()
		sub(&exchange.Cash, total)
		if releaseHold.IsPositive() {
			order.Lock.RLock()
			cumulativeSpent := order.Notional.Load().Add(order.Fee.Load())
			order.Lock.RUnlock()
			// Release unused buying power back
			unusedHold := releaseHold.Sub(cumulativeSpent)
			if unusedHold.IsPositive() {
				add(&exchange.DayTradingBuyingPower, unusedHold)
			}
		} else if holdAlreadyReleased {
			sub(&exchange.DayTradingBuyingPower, total)
		}
		exchange.Lock.Unlock()

		// Add shares to holding
		eq.Shares.Lock.Lock()
		add(&eq.Shares.Available, actualFilled)
		add(&eq.Shares.Quantity, actualFilled)
		eq.Shares.Volume += actualFilled.Float64()
		eq.Shares.BuyVolume += actualFilled.Float64()
		eq.Shares.Lots.Add(now, actualFilled, total)
		eq.Shares.Check()
		eq.Shares.Lock.Unlock()

	case ds.SideSell:
		// Remove shares from holding
		eq.Shares.Lock.Lock()
		sub(&eq.Shares.Quantity, actualFilled)
		if holdAlreadyReleased {
			sub(&eq.Shares.Available, actualFilled)
		}
		eq.Shares.Volume += actualFilled.Float64()
		eq.Shares.SellVolume += actualFilled.Float64()
		costBasis := eq.Shares.Lots.Consume(actualFilled, decimal.Zero)
		// Track wins/losses
		profit := total.Sub(costBasis)
		if profit.IsPositive() {
			eq.Shares.WinCount++
		} else if profit.IsNegative() {
			eq.Shares.LossCount++
		}
		eq.Shares.Check()
		eq.Shares.Lock.Unlock()

		// Add proceeds to exchange cash
		exchange.Lock.Lock()
		add(&exchange.Cash, total)
		// Restore buying power: proceeds go back to available buying power
		// The buying power we get back is the sale proceeds (we can re-invest them)
		add(&exchange.DayTradingBuyingPower, total)
		exchange.Lock.Unlock()
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
	exchange.Fees = exchange.Fees.Add(actualFee)
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
