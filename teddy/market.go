package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"fmt"
)

// MarketOrder places a market order on the exchange or simulates one in backtest mode.
func (p *Pair) MarketOrder(side ds.Side, quantity decimal.Decimal) (*Order, error) {
	exchange := p.Exchange
	orders := exchange.Orders
	holdings := exchange.Holdings
	exchange.Lock.RLock()
	feeRate := exchange.TakerFee
	exchange.Lock.RUnlock()

	// check order parameters
	p.Lock.RLock()
	quoteMinSize := p.QuoteMinSize
	quoteMaxSize := p.QuoteMaxSize
	if quantity.Cmp(p.BaseMinSize) < 0 {
		err := fmt.Errorf("quantity %s %s is below minimum size of %s %s", quantity, p.BaseCurrency, p.BaseMinSize, p.BaseCurrency)
		p.Lock.RUnlock()
		return nil, err
	}
	if quantity.Cmp(p.BaseMaxSize) > 0 {
		err := fmt.Errorf("quantity %s %s is above maximum size of %s %s", quantity, p.BaseCurrency, p.BaseMaxSize, p.BaseCurrency)
		p.Lock.RUnlock()
		return nil, err
	}
	if quantity.Quantize(p.BaseIncrement).Cmp(quantity) != 0 {
		err := fmt.Errorf("quantity %s %s is not a multiple of increment %s %s", quantity, p.BaseCurrency, p.BaseIncrement, p.BaseCurrency)
		p.Lock.RUnlock()
		return nil, err
	}
	p.Lock.RUnlock()

	// get national best asking price
	bestBid, bestAsk := p.OrderBook.BestBidAsk()
	switch side {
	case ds.SideBuy:
		if bestAsk.Mul(quantity).Cmp(quoteMinSize) < 0 {
			err := fmt.Errorf("order quantity %s %s too small", quantity, p.BaseCurrency)
			return nil, err
		}
		if bestAsk.Mul(quantity).Cmp(quoteMaxSize) > 0 {
			err := fmt.Errorf("order quantity %s %s too large", quantity, p.BaseCurrency)
			return nil, err
		}
	case ds.SideSell:
		if bestBid.Mul(quantity).Cmp(quoteMinSize) < 0 {
			err := fmt.Errorf("order quantity %s %s too small", quantity, p.BaseCurrency)
			return nil, err
		}
		if bestBid.Mul(quantity).Cmp(quoteMaxSize) > 0 {
			err := fmt.Errorf("order quantity %s %s too large", quantity, p.BaseCurrency)
			return nil, err
		}
	}

	holdAmount := decimal.Zero
	buyingPower := decimal.Zero
	baseHolding := holdings.Get(p.BaseCurrency)
	quoteHolding := holdings.Get(p.QuoteCurrency)
	switch side {
	case ds.SideBuy:
		// put on hold a decent amount of cash for market order
		// coinbase policy prevents market orders from slipping more than 10%
		// https://help.coinbase.com/en/coinbase/trading-and-funding/advanced-trade/order-types#market-order
		quoteHolding.Lock.Lock()
		maxCash := quoteHolding.Available.Mul(decimal.One.Sub(feeRate))
		maxValue := quantity.Mul(bestAsk).Mul(decimal.Parse("1.10"))
		buyingPower = maxValue.Min(maxCash).Min(quoteMaxSize)
		holdAmount = buyingPower.Add(buyingPower.Mul(feeRate))
		if holdAmount.Cmp(quoteHolding.Available) > 0 {
			loggy.Fatalf("buying power computation broken: holdAmount %s > available %s", holdAmount, quoteHolding.Available)
		}
		if buyingPower.Cmp(quoteMinSize) < 0 {
			quoteHolding.Lock.Unlock()
			return nil, ds.ErrInsufficientFunds
		}
		quoteHolding.Available = quoteHolding.Available.Sub(holdAmount)
		quoteHolding.Lock.Unlock()

	case ds.SideSell:
		// put on hold the full intended amount of asset
		baseHolding.Lock.Lock()
		if quantity.Cmp(baseHolding.Available) > 0 {
			err := fmt.Errorf("tried to sell %s %s but only have %s %s available", quantity, p.BaseCurrency, baseHolding.Available, p.BaseCurrency)
			baseHolding.Lock.Unlock()
			return nil, err
		}
		baseHolding.Available = baseHolding.Available.Sub(quantity)
		baseHolding.Lock.Unlock()
		holdAmount = quantity
	}

	// live trade market order
	if Live {
		switch p.Exchange.Exchange {
		case ds.ExchangeCoinbase:
			order := p.Exchange.Orders.create(p, ds.OrderTypeMarket, side, quantity, decimal.Zero, decimal.Zero)
			orderID, err := CoinbaseClient.MarketOrder(p.Symbol(), side, quantity, order.ClientOrderID, GetCostBasisMethod())
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
			loggy.Fatalf("market orders not supported on %v", p.Exchange)
		}
	}

	// simulate ioc market buy order
	var fills []ds.Level
	var filledQuantity decimal.Decimal
	switch side {
	case ds.SideBuy:
		filledQuantity, fills = p.OrderBook.Buy(quantity, buyingPower)
	case ds.SideSell:
		filledQuantity, fills = p.OrderBook.Sell(quantity)
	}
	if fills == nil {
		quoteHolding.Lock.Lock()
		quoteHolding.Available = quoteHolding.Available.Add(holdAmount)
		quoteHolding.Lock.Unlock()
		return nil, fmt.Errorf("insufficient liquidity for market buy of %s %s", quantity, p.BaseCurrency)
	}

	// create order and generate fills
	order := orders.create(p, ds.OrderTypeMarket, ds.SideBuy, filledQuantity, decimal.Zero, holdAmount)
	for _, fill := range fills {
		fillValue := fill.Price.Mul(fill.Size)
		order.fill(fill.Size, fillValue)
	}
	return order, nil
}
