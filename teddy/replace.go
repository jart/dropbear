package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"log"
)

// Replace atomically replaces an existing order with new quantity and price.
// This is used to update stink bids when the market price moves.
func (o *Order) Replace(newQty, newPrice decimal.Decimal) error {
	pair := o.Pair
	broker := pair.Broker

	// validate order can be replaced
	o.Lock.RLock()
	if o.OrderID == "" {
		o.Lock.RUnlock()
		return ds.ErrNotFound
	}
	if o.State.Load().IsFinal() {
		o.Lock.RUnlock()
		return ds.ErrOrderNotOpen
	}
	oldQty := o.Quantity.Load()
	oldPrice := o.LimitPrice.Load()
	oldHold := o.Hold.Load()
	side := o.Side
	o.Lock.RUnlock()

	// compute new hold requirement
	var newHold decimal.Decimal
	switch side {
	case ds.SideBuy:
		newNotional := newQty.Mul(newPrice)
		maxFee := newNotional.Mul(broker.TakerFee.Load())
		newHold = newNotional.Add(maxFee)
	case ds.SideSell:
		newHold = newQty
	}

	// adjust available balance
	holdDelta := newHold.Sub(oldHold)
	switch side {
	case ds.SideBuy:
		pair.QuoteCurrency.Lock.Lock()
		available := pair.QuoteCurrency.Available.Load()
		if holdDelta.IsPositive() && holdDelta.Cmp(available) > 0 {
			pair.QuoteCurrency.Lock.Unlock()
			if *flagVerbose {
				log.Printf("[teddy] replace needs %s %s additional hold but only %s available",
					holdDelta, pair.QuoteCurrency, available)
			}
			return ds.ErrInsufficientFunds
		}
		pair.QuoteCurrency.Available.Store(available.Sub(holdDelta))
		pair.QuoteCurrency.Check()
		pair.QuoteCurrency.Lock.Unlock()
	case ds.SideSell:
		pair.BaseCurrency.Lock.Lock()
		available := pair.BaseCurrency.Available.Load()
		if holdDelta.IsPositive() && holdDelta.Cmp(available) > 0 {
			pair.BaseCurrency.Lock.Unlock()
			if *flagVerbose {
				log.Printf("[teddy] replace needs %s %s additional hold but only %s available",
					holdDelta, pair.BaseCurrency, available)
			}
			return ds.ErrInsufficientFunds
		}
		pair.BaseCurrency.Available.Store(available.Sub(holdDelta))
		pair.BaseCurrency.Check()
		pair.BaseCurrency.Lock.Unlock()
	}

	// call broker to replace the order
	if !Paper {
		switch broker.Broker {
		case ds.BrokerCoinbase:
			if err := CoinbaseClient.ReplaceOrder(o.OrderID, newQty, newPrice); err != nil {
				// roll back available balance change
				switch side {
				case ds.SideBuy:
					pair.QuoteCurrency.Lock.Lock()
					add(&pair.QuoteCurrency.Available, holdDelta)
					pair.QuoteCurrency.Check()
					pair.QuoteCurrency.Lock.Unlock()
				case ds.SideSell:
					pair.BaseCurrency.Lock.Lock()
					add(&pair.BaseCurrency.Available, holdDelta)
					pair.BaseCurrency.Check()
					pair.BaseCurrency.Lock.Unlock()
				}
				// if order was filled/cancelled during replace, kill it locally
				if err == ds.ErrNotFound || err == ds.ErrOrderNotOpen {
					o.kill(ds.OrderStateCanceled)
				}
				return err
			}
		default:
			loggy.Fatalf("replace not supported on %v", broker)
		}
	}

	// update local order state
	o.Lock.Lock()
	o.Quantity.Store(newQty)
	o.LimitPrice.Store(newPrice)
	o.Hold.Store(newHold)
	o.Lock.Unlock()

	if *flagVerbose {
		log.Printf("[teddy] replaced %s order: %s @ $%s -> %s @ $%s",
			side, oldQty, oldPrice, newQty, newPrice)
	}

	return nil
}
