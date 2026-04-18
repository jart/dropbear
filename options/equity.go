package options

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
)

type Equity struct {
	Symbol  symbol.Symbol   // ticker symbol, e.g. "SPY"
	Bid     decimal.Decimal // bid price, e.g. 0.10 (or zero if undefined)
	Ask     decimal.Decimal // ask price, e.g. 0.15 (or zero if undefined)
	BidSize uint32          // number of contracts available at the best bid price
	AskSize uint32          // number of contracts available at the best ask price
	ID      uint32          // instrument id
	TS      clocky.Time     // timestamp of this quote
}

func (e *Equity) String() string            { return e.GetName() }
func (e *Equity) GetName() string           { return e.Symbol.String() }
func (e *Equity) GetSymbol() symbol.Symbol  { return e.Symbol }
func (e *Equity) GetID() uint32             { return e.ID }
func (e *Equity) GetBid() decimal.Decimal   { return e.Bid }
func (e *Equity) GetAsk() decimal.Decimal   { return e.Ask }
func (e *Equity) GetBidSize() uint32        { return e.BidSize }
func (e *Equity) GetAskSize() uint32        { return e.AskSize }
func (e *Equity) MidPrice() decimal.Decimal { return e.Bid.Add(e.Ask).Half() }
func (e *Equity) Multiplier() int           { return 1 }
func (e *Equity) GetDelta() decimal.Decimal { return decimal.One }

func (e *Equity) IntrinsicValue(underlyingPrice decimal.Decimal) decimal.Decimal {
	return underlyingPrice
}
