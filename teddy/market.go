package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"errors"
)

// slippage is the maximum price movement allowed for market orders.
// coinbase policy prevents market orders from slipping more than 10%.
// https://help.coinbase.com/en/coinbase/trading-and-funding/advanced-trade/order-types#market-order
const slippage = decimal.Tenth

// MarketOrder places a market order as an IOC limit order with 10% slippage.
func (p *Pair) MarketOrder(side ds.Side, quantity decimal.Decimal) (*Order, error) {
	if !p.LastPrice.IsPositive() {
		return nil, errors.New("market order failed: price not initialized")
	}
	limitPrice := p.LastPrice.Mul(decimal.One.Add(slippage.Mul(decimal.Decimal(side))))
	limitPrice = limitPrice.QuantizeTruncate(p.QuoteIncrement.Load())
	return p.LimitOrder(side, quantity, limitPrice, ds.OrderStrategyIOC)
}
