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

	// Short selling flags
	IsShortSale bool // true if this is a short sale (sell short)
	IsCover     bool // true if this is a buy-to-cover
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
	broker := eq.Broker
	orders := broker.Orders
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
			broker.Lock.Lock()
			add(&broker.DayTradingBuyingPower, releaseAmount)
			broker.Lock.Unlock()
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
	broker := eq.Broker
	orders := broker.Orders
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

	// Calculate price for potential recalculations
	price := actualNotional.Div(actualFilled)

	// Log the fill (if verbose)
	if Live || *flagVerbose {
		log.Printf("%s %s %s @ %s ($%s)",
			order.Side.String(), actualFilled.String(), eq.Symbol,
			price.Format(2), actualNotional.Format(2))
	}

	switch {
	case order.IsCover:
		// Buy to cover: pay cash, close short position
		// Check current position first - it may have changed since order was placed
		eq.Shares.Lock.Lock()
		currentQty := eq.Shares.Quantity.Load()
		if !currentQty.IsNegative() {
			// Position already closed or is long - nothing to cover
			eq.Shares.Lock.Unlock()
			return decimal.Zero, nil
		}
		absQty := currentQty.Neg()
		if actualFilled.Cmp(absQty) > 0 {
			// Cap at actual short position size
			actualFilled = absQty
			actualNotional = actualFilled.Mul(price)
			total = actualNotional.Add(fee) // Cover costs notional + fee
		}
		eq.Shares.Lock.Unlock()

		broker.Lock.Lock()
		sub(&broker.Cash, total)
		if releaseHold.IsPositive() {
			order.Lock.RLock()
			cumulativeSpent := order.Notional.Load().Add(order.Fee.Load())
			order.Lock.RUnlock()
			unusedHold := releaseHold.Sub(cumulativeSpent)
			if unusedHold.IsPositive() {
				add(&broker.DayTradingBuyingPower, unusedHold)
			}
		} else if holdAlreadyReleased {
			sub(&broker.DayTradingBuyingPower, total)
		}
		broker.Lock.Unlock()

		// Close short position (increase quantity toward zero)
		eq.Shares.Lock.Lock()
		add(&eq.Shares.Quantity, actualFilled) // -100 + 50 = -50
		eq.Shares.Volume += actualFilled.Float64()
		eq.Shares.BuyVolume += actualFilled.Float64()
		// Consume from ShortLots to get proceeds (cost basis for shorts)
		shortProceeds := eq.Shares.ShortLots.Consume(actualFilled, decimal.Zero)
		// P&L = proceeds from short sale - cost to cover
		// profit if shortProceeds > total (sold high, bought low)
		profit := shortProceeds.Sub(total)
		if profit.IsPositive() {
			eq.Shares.WinCount++
		} else if profit.IsNegative() {
			eq.Shares.LossCount++
		}
		eq.Shares.Check()
		eq.Shares.Lock.Unlock()

	case order.IsShortSale:
		// Short sale: receive cash, create short position
		eq.Shares.Lock.Lock()
		sub(&eq.Shares.Quantity, actualFilled) // 0 - 100 = -100 (short)
		eq.Shares.Volume += actualFilled.Float64()
		eq.Shares.SellVolume += actualFilled.Float64()
		// Track short proceeds in ShortLots (this is our "cost basis" for the short)
		eq.Shares.ShortLots.Add(now, actualFilled, total)
		eq.Shares.Check()
		eq.Shares.Lock.Unlock()

		// Add proceeds to broker cash
		broker.Lock.Lock()
		add(&broker.Cash, total)
		// Release margin hold and restore appropriate buying power
		if releaseHold.IsPositive() {
			// Short sales use margin, not buying power directly
			// The proceeds don't add to buying power the same way
		}
		broker.Lock.Unlock()

	case order.Side == ds.SideBuy:
		// Regular buy: deduct cash, add shares
		broker.Lock.Lock()
		sub(&broker.Cash, total)
		if releaseHold.IsPositive() {
			order.Lock.RLock()
			cumulativeSpent := order.Notional.Load().Add(order.Fee.Load())
			order.Lock.RUnlock()
			// Release unused buying power back
			unusedHold := releaseHold.Sub(cumulativeSpent)
			if unusedHold.IsPositive() {
				add(&broker.DayTradingBuyingPower, unusedHold)
			}
		} else if holdAlreadyReleased {
			sub(&broker.DayTradingBuyingPower, total)
		}
		broker.Lock.Unlock()

		// Add shares to holding
		eq.Shares.Lock.Lock()
		add(&eq.Shares.Available, actualFilled)
		add(&eq.Shares.Quantity, actualFilled)
		eq.Shares.Volume += actualFilled.Float64()
		eq.Shares.BuyVolume += actualFilled.Float64()
		eq.Shares.Lots.Add(now, actualFilled, total)
		eq.Shares.Check()
		eq.Shares.Lock.Unlock()

	case order.Side == ds.SideSell:
		// Regular sell: remove shares, add cash
		eq.Shares.Lock.Lock()

		// Check current position - it may have changed since order was placed
		// (e.g., margin call liquidation)
		currentQty := eq.Shares.Quantity.Load()
		if !currentQty.IsPositive() {
			// Position already closed or short - nothing to sell
			eq.Shares.Lock.Unlock()
			return decimal.Zero, nil
		}
		if actualFilled.Cmp(currentQty) > 0 {
			// Cap at available shares
			actualFilled = currentQty
			actualNotional = actualFilled.Mul(price)
			total = actualNotional.Sub(fee)
		}

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

		// Add proceeds to broker cash
		broker.Lock.Lock()
		add(&broker.Cash, total)
		// Restore buying power: proceeds go back to available buying power
		add(&broker.DayTradingBuyingPower, total)
		broker.Lock.Unlock()
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

	broker.Lock.Lock()
	broker.Fees = broker.Fees.Add(actualFee)
	broker.Lock.Unlock()
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
