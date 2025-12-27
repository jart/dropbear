package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"fmt"
	"log"
)

// LimitOrder places a limit order on the exchange or simulates one in backtest mode.
func (p *Pair) LimitOrder(side ds.Side, quantity, limitPrice decimal.Decimal, strategy ds.OrderStrategy) (*Order, error) {

	// compute notional value
	notional := quantity.Mul(limitPrice)
	switch side {
	case ds.SideBuy:
		notional = notional.QuantizeUp(p.QuoteIncrement.Load())
	case ds.SideSell:
		notional = notional.QuantizeDown(p.QuoteIncrement.Load())
	}

	// validate order parameters
	if err := p.checkLimitOrderParams(side, quantity, limitPrice, notional); err != nil {
		return nil, err
	}

	// post-only orders aren't allowed to cross the spread
	if strategy == ds.OrderStrategyPostOnly {
		bestBid, bestAsk := p.OrderBook.BestBidAsk()
		switch side {
		case ds.SideBuy:
			if limitPrice.Cmp(bestAsk) >= 0 {
				return nil, ds.ErrPostOnly
			}
		case ds.SideSell:
			if limitPrice.Cmp(bestBid) <= 0 {
				return nil, ds.ErrPostOnly
			}
		}
	}

	// simulate rate limits in backtest mode
	// the coinbase client library limits in prod
	if Paper {
		if !gRateLimiter.Try() {
			return nil, ds.ErrTooManyRequests
		}
	}

	// put hold on account
	hold := decimal.Zero
	switch side {
	case ds.SideBuy:
		// put on hold enough cash to pay maximum cost
		maxFee := notional.Mul(p.Exchange.TakerFee.Load())
		maxCost := notional.Add(maxFee)
		p.QuoteCurrency.Lock.Lock()
		available := p.QuoteCurrency.Available.Load()
		if maxCost.Cmp(available) > 0 {
			p.QuoteCurrency.Lock.Unlock()
			if *flagVerbose {
				log.Printf("[teddy] buying %s %s @ $%s for %s %s requires %s %s hold but only had %s %s available",
					quantity, p.BaseCurrency, limitPrice, notional, p.QuoteCurrency, maxCost, p.QuoteCurrency, available, p.QuoteCurrency)
			}
			return nil, ds.ErrInsufficientFunds
		}
		p.QuoteCurrency.Available.Store(available.Sub(maxCost))
		p.QuoteCurrency.Check()
		p.QuoteCurrency.Lock.Unlock()
		hold = maxCost
	case ds.SideSell:
		// put on hold enough coin to fill order
		p.BaseCurrency.Lock.Lock()
		available := p.BaseCurrency.Available.Load()
		if quantity.Cmp(available) > 0 {
			p.BaseCurrency.Lock.Unlock()
			if *flagVerbose {
				log.Printf("[teddy] selling %s %s @ $%s requires %s %s hold but only had %s %s available",
					quantity, p.BaseCurrency, limitPrice, quantity, p.BaseCurrency, available, p.BaseCurrency)
			}
			return nil, ds.ErrInsufficientFunds
		}
		p.BaseCurrency.Available.Store(available.Sub(quantity))
		p.BaseCurrency.Check()
		p.BaseCurrency.Lock.Unlock()
		hold = quantity
	}

	// create order object
	order := p.Exchange.Orders.create(p, ds.OrderTypeLimit, side, quantity, limitPrice, hold)

	// handle live trading
	if !Paper {
		switch p.Exchange.Exchange {
		case ds.ExchangeCoinbase:
			orderID, err := CoinbaseClient.LimitOrder(p.Symbol, side, quantity, limitPrice, order.ClientOrderID, strategy, GetCostBasisMethod())
			if orderID != "" {
				p.Exchange.Orders.lock.Lock()
				order.OrderID = orderID
				p.Exchange.Orders.ordersMap[orderID] = order
				p.Exchange.Orders.lock.Unlock()
			}
			if err != nil {
				order.kill(ds.OrderStateInvalid)
				return nil, err
			}
			return order, nil
		default:
			loggy.Fatalf("orders not supported on %v", p.Exchange)
		}
	}

	// simulate limit order
	orderID := GenerateOrderID()
	order.Lock.Lock()
	order.OrderID = orderID
	order.Lock.Unlock()

	// marketable limit orders may take liquidity by crossing the spread
	switch strategy {
	case ds.OrderStrategyMarketable, ds.OrderStrategyIOC:
		remaining := quantity
		for remaining.IsPositive() {
			fill, ok := p.OrderBook.Pop(side, remaining, limitPrice)
			if !ok {
				break
			}
			fillNotional := fill.Price.Mul(fill.Size)
			filled, err := order.fill(fill.Size, fillNotional, p.Exchange.TakerFee, false)
			if unfilled := fill.Size.Sub(filled); unfilled.IsPositive() {
				p.OrderBook.Push(side.Flip(), fill.Price, unfilled)
			}
			if err != nil || !filled.IsPositive() {
				break
			}
			remaining = remaining.Sub(filled)
		}
		if remaining.IsZero() {
			return order, nil
		}
		if strategy == ds.OrderStrategyIOC {
			order.kill(ds.OrderStateCanceled)
			return order, nil
		}
		quantity = remaining
	}

	// in live mode the exchange tracks the order; in backtest mode we just
	// track it in openOrders and let handleTick simulate fills by popping
	// from the book when trades happen at our price level
	return order, nil
}

func (p *Pair) checkLimitOrderParams(side ds.Side, quantity, limitPrice, notional decimal.Decimal) error {
	p.Lock.RLock()
	defer p.Lock.RUnlock()

	// check quantity
	if quantity.Cmp(p.BaseMinSize.Load()) < 0 {
		return fmt.Errorf("quantity %s %s is below minimum size of %s %s",
			quantity, p.BaseCurrency, p.BaseMinSize.Load(), p.BaseCurrency)
	}
	if quantity.Cmp(p.BaseMaxSize.Load()) > 0 {
		return fmt.Errorf("quantity %s %s is above maximum size of %s %s",
			quantity, p.BaseCurrency, p.BaseMaxSize.Load(), p.BaseCurrency)
	}

	// check notional value
	if notional.Cmp(p.QuoteMinSize.Load()) < 0 {
		return fmt.Errorf("notional %s %s is below minimum size of %s %s",
			notional, p.QuoteCurrency, p.QuoteMinSize.Load(), p.QuoteCurrency)
	}
	if notional.Cmp(p.QuoteMaxSize.Load()) > 0 {
		return fmt.Errorf("notional %s %s is above maximum size of %s %s",
			notional, p.QuoteCurrency, p.QuoteMaxSize.Load(), p.QuoteCurrency)
	}

	// check rounding
	if quantity.Quantize(p.BaseIncrement.Load()).Cmp(quantity) != 0 {
		return fmt.Errorf("quantity %s %s is not a multiple of increment %s %s",
			quantity, p.BaseCurrency, p.BaseIncrement.Load(), p.BaseCurrency)
	}
	if limitPrice.Quantize(p.QuoteIncrement.Load()).Cmp(limitPrice) != 0 {
		return fmt.Errorf("limitPrice %s %s is not a multiple of increment %s %s",
			limitPrice, p.QuoteCurrency, p.QuoteIncrement.Load(), p.QuoteCurrency)
	}

	// self-trading is insane because it appears ot be one of the most legitimately dangerous
	// practices with the potential to gigafry your brokerage account but is exclusively done
	// by literal turbonormies who unironically want to "improve user engagement metrics" and
	// basically get oneshotted by regulators.
	for it := p.openOrders.Iterator(); it.Next(); {
		order := it.Value()
		limitPrice := order.LimitPrice.Load()
		if order.Side == side.Flip() && limitPrice.Mul(decimal.Decimal(side)).Cmp(limitPrice) >= 0 {
			return ds.ErrSelfTrade
		}
	}

	return nil
}
