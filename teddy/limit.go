package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"fmt"
)

// LimitOrder places a limit order on the exchange or simulates one in backtest mode.
func (p *Pair) LimitOrder(side ds.Side, quantity, limitPrice decimal.Decimal, strategy ds.LimitOrderStrategy) (*Order, error) {
	exchange := p.Exchange
	exchange.Lock.RLock()
	feeRate := exchange.TakerFee
	rebateRate := exchange.Rebate
	exchange.Lock.RUnlock()
	notional := quantity.Mul(limitPrice)

	// perform sanity checks
	if err := p.checkSelfTrade(side, limitPrice); err != nil {
		return nil, err
	}
	if err := p.precheckLimitOrder(quantity, limitPrice, notional, feeRate, rebateRate); err != nil {
		return nil, err
	}

	// post-only orders aren't allowed to cross the spread
	if strategy == ds.LimitOrderStrategyPostOnly {
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
		baseHolding.Lock.Unlock()
		hold = quantity
	}

	// create order object
	order := p.Exchange.Orders.create(p, ds.OrderTypeLimit, side, quantity, limitPrice, hold)

	// handle live trading
	if Live {
		switch p.Exchange.Exchange {
		case ds.ExchangeCoinbase:
			orderID, err := CoinbaseClient.LimitOrder(p.Symbol(), side, quantity, limitPrice, "", strategy, GetCostBasisMethod())
			if err != nil {
				order.kill(ds.OrderStateInvalid)
				return nil, err
			}
			p.Exchange.Orders.lock.Lock()
			order.OrderID = orderID
			p.Exchange.Orders.ordersMap[orderID] = order
			p.Exchange.Orders.lock.Unlock()
			return order, nil
		default:
			loggy.Fatalf("limit orders not supported on %v", p.Exchange)
		}
	}

	// simulate limit order
	orderID := GenerateOrderID()
	order.Lock.Lock()
	order.OrderID = orderID
	order.Lock.Unlock()

	// marketable limit orders may take liquidity by crossing the spread
	switch strategy {
	case ds.LimitOrderStrategyMarketable, ds.LimitOrderStrategyIOC:
		takenQuantity, fills := p.OrderBook.BuyLimit(quantity, limitPrice)
		if fills != nil {
			for _, fill := range fills {
				fillValue := fill.Price.Mul(fill.Size)
				order.fill(fill.Size, fillValue)
			}
			if takenQuantity.Cmp(quantity) == 0 {
				if order.State != ds.OrderStateFilled {
					loggy.Fatalf("limit order should be filled but was %s", order.State)
				}
				return order, nil
			}
			quantity = quantity.Sub(takenQuantity)
		}
	}

	// order goes on book for remaining quantity
	p.OrderBook.Add(ds.SideBuy, limitPrice, quantity)
	return order, nil
}

func (p *Pair) precheckLimitOrder(quantity, limitPrice, notional, feeRate, rebateRate decimal.Decimal) error {
	p.Lock.RLock()
	defer p.Lock.RUnlock()
	if quantity.Cmp(p.BaseMinSize) < 0 {
		return fmt.Errorf("quantity %s %s is below minimum size of %s %s", quantity, p.BaseCurrency, p.BaseMinSize, p.BaseCurrency)
	}
	if quantity.Cmp(p.BaseMaxSize) > 0 {
		return fmt.Errorf("quantity %s %s is above maximum size of %s %s", quantity, p.BaseCurrency, p.BaseMaxSize, p.BaseCurrency)
	}
	if quantity.Quantize(p.BaseIncrement).Cmp(quantity) != 0 {
		return fmt.Errorf("quantity %s %s is not a multiple of increment %s %s", quantity, p.BaseCurrency, p.BaseIncrement, p.BaseCurrency)
	}
	if limitPrice.Quantize(p.QuoteIncrement).Cmp(limitPrice) != 0 {
		return fmt.Errorf("limitPrice %s %s is not a multiple of increment %s %s", limitPrice, p.QuoteCurrency, p.QuoteIncrement, p.QuoteCurrency)
	}
	if notional.Cmp(p.QuoteMinSize) < 0 {
		return fmt.Errorf("notional %s %s is below minimum size of %s %s", notional, p.QuoteCurrency, p.QuoteMinSize, p.QuoteCurrency)
	}
	if notional.Cmp(p.QuoteMaxSize) > 0 {
		return fmt.Errorf("notional %s %s is above maximum size of %s %s", notional, p.QuoteCurrency, p.QuoteMaxSize, p.QuoteCurrency)
	}
	return nil
}
