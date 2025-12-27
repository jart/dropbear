package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"fmt"
)

// LimitOrder places a limit order on the exchange or simulates one in backtest mode.
func (p *Pair) LimitOrder(side ds.Side, quantity, limitPrice decimal.Decimal, strategy ds.OrderStrategy) (*Order, error) {
	exchange := p.Exchange
	orders := exchange.Orders
	feeRate := exchange.TakerFee
	notional := quantity.Mul(limitPrice)

	// sanity check order parameters
	if quantity.Cmp(p.BaseMinSize) < 0 {
		return nil, fmt.Errorf("quantity %s %s is below minimum size of %s %s",
			quantity, p.BaseCurrency, p.BaseMinSize, p.BaseCurrency)
	}
	if quantity.Cmp(p.BaseMaxSize) > 0 {
		return nil, fmt.Errorf("quantity %s %s is above maximum size of %s %s",
			quantity, p.BaseCurrency, p.BaseMaxSize, p.BaseCurrency)
	}
	if quantity.Quantize(p.BaseIncrement).Cmp(quantity) != 0 {
		return nil, fmt.Errorf("quantity %s %s is not a multiple of increment %s %s",
			quantity, p.BaseCurrency, p.BaseIncrement, p.BaseCurrency)
	}
	if limitPrice.Quantize(p.QuoteIncrement).Cmp(limitPrice) != 0 {
		return nil, fmt.Errorf("limitPrice %s %s is not a multiple of increment %s %s",
			limitPrice, p.QuoteCurrency, p.QuoteIncrement, p.QuoteCurrency)
	}
	if notional.Cmp(p.QuoteMinSize) < 0 {
		return nil, fmt.Errorf("notional %s %s is below minimum size of %s %s",
			notional, p.QuoteCurrency, p.QuoteMinSize, p.QuoteCurrency)
	}
	if notional.Cmp(p.QuoteMaxSize) > 0 {
		return nil, fmt.Errorf("notional %s %s is above maximum size of %s %s",
			notional, p.QuoteCurrency, p.QuoteMaxSize, p.QuoteCurrency)
	}

	// self-trading is insane because it appears ot be one of the most legitimately dangerous
	// practices with the potential to gigafry your brokerage account but is exclusively done
	// by literal turbonormies who unironically want to "improve user engagement metrics" and
	// basically get oneshotted by regulators.
	for it := p.openOrders.Iterator(); it.Next(); {
		order := it.Value()
		if order.Side == side.Flip() && limitPrice.Mul(decimal.Decimal(side)).Cmp(order.LimitPrice) >= 0 {
			return nil, ds.ErrSelfTrade
		}
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
	if !Live {
		if !gRateLimiter.Try() {
			return nil, ds.ErrTooManyRequests
		}
	}

	// put hold on account
	hold := decimal.Zero
	switch side {
	case ds.SideBuy:
		// put on hold enough cash to pay maximum cost
		maxFee := notional.Mul(feeRate)
		maxCost := notional.Add(maxFee)
		quoteHolding := p.Exchange.Holdings.Get(p.QuoteCurrency)
		quoteHolding.Lock.Lock()
		if maxCost.Cmp(quoteHolding.Available) > 0 {
			quoteHolding.Lock.Unlock()
			return nil, ds.ErrInsufficientFunds
		}
		quoteHolding.Available = quoteHolding.Available.Sub(maxCost)
		quoteHolding.Check()
		quoteHolding.Lock.Unlock()
		hold = maxCost
	case ds.SideSell:
		// put on hold enough coin to fill order
		baseHolding := p.Exchange.Holdings.Get(p.BaseCurrency)
		baseHolding.Lock.Lock()
		if quantity.Cmp(baseHolding.Available) > 0 {
			baseHolding.Lock.Unlock()
			return nil, ds.ErrInsufficientFunds
		}
		baseHolding.Available = baseHolding.Available.Sub(quantity)
		baseHolding.Check()
		baseHolding.Lock.Unlock()
		hold = quantity
	}

	// create order object
	order := orders.create(p, ds.OrderTypeLimit, side, quantity, limitPrice, hold)

	// handle live trading
	if Live {
		switch p.Exchange.Exchange {
		case ds.ExchangeCoinbase:
			orderID, err := CoinbaseClient.LimitOrder(p.Symbol, side, quantity, limitPrice, order.ClientOrderID, strategy, GetCostBasisMethod())
			if orderID != "" {
				orders.lock.Lock()
				order.OrderID = orderID
				orders.ordersMap[orderID] = order
				orders.lock.Unlock()
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
			fillValue := fill.Price.Mul(fill.Size)
			filled, err := order.fill(fill.Size, fillValue, exchange.TakerFee, false)
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
