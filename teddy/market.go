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
	feeRate := exchange.TakerFee
	holdings := exchange.Holdings

	// check order parameters
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

	// get national best asking price
	bestNotional := decimal.Zero
	bestBid, bestAsk := p.OrderBook.BestBidAsk()
	switch side {
	case ds.SideBuy:
		bestNotional = bestAsk.Mul(quantity)
	case ds.SideSell:
		bestNotional = bestBid.Mul(quantity)
	}
	if bestNotional.Cmp(p.QuoteMinSize) < 0 {
		err := fmt.Errorf("order quantity %s %s too small", quantity, p.BaseCurrency)
		return nil, err
	}
	if bestNotional.Cmp(p.QuoteMaxSize) > 0 {
		err := fmt.Errorf("order quantity %s %s too large", quantity, p.BaseCurrency)
		return nil, err
	}

	// prepare to place hold
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
		buyingPower = maxValue.Min(maxCash).Min(p.QuoteMaxSize)
		holdAmount = buyingPower.Add(buyingPower.Mul(feeRate))
		if holdAmount.Cmp(quoteHolding.Available) > 0 {
			loggy.Fatalf("buying power computation broken: holdAmount %s > available %s", holdAmount, quoteHolding.Available)
		}
		if buyingPower.Cmp(p.QuoteMinSize) < 0 {
			quoteHolding.Lock.Unlock()
			return nil, ds.ErrInsufficientFunds
		}
		quoteHolding.Available = quoteHolding.Available.Sub(holdAmount)
		quoteHolding.Lock.Unlock()
	case ds.SideSell:
		// put on hold the full intended amount of asset
		baseHolding.Lock.Lock()
		if quantity.Cmp(baseHolding.Available) > 0 {
			err := fmt.Errorf("tried to sell %s %s but only have %s %s available",
				quantity, p.BaseCurrency, baseHolding.Available, p.BaseCurrency)
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
			// pass our holdAmount so order.fill() can release it properly
			order := orders.create(p, ds.OrderTypeMarket, side, quantity, decimal.Zero, holdAmount)
			orderID, err := CoinbaseClient.MarketOrder(p.Symbol(), side, quantity, order.ClientOrderID, GetCostBasisMethod())
			if err != nil {
				order.kill(ds.OrderStateInvalid)
				return nil, err
			}
			orders.lock.Lock()
			order.OrderID = orderID
			orders.ordersMap[orderID] = order
			orders.lock.Unlock()
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
		quoteHolding.Check()
		quoteHolding.Lock.Unlock()
		return nil, fmt.Errorf("insufficient liquidity for market buy of %s %s", quantity, p.BaseCurrency)
	}

	// create order and generate fills
	order := orders.create(p, ds.OrderTypeMarket, side, filledQuantity, decimal.Zero, holdAmount)
	for _, fill := range fills {
		fillValue := fill.Price.Mul(fill.Size)
		order.fill(fill.Size, fillValue, feeRate)
	}
	return order, nil
}
