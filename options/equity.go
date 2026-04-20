package options

import (
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
)

type Equity struct {
	Symbol symbol.Symbol // ticker symbol, e.g. "SPY"
	Book   Book          // order book for this equity
	ID     uint32        // instrument id
	TS     clocky.Time   // timestamp of this quote
}

func (e *Equity) String() string            { return e.Name() }
func (e *Equity) Name() string              { return e.Symbol.String() }
func (e *Equity) GetSymbol() symbol.Symbol  { return e.Symbol }
func (e *Equity) GetID() uint32             { return e.ID }
func (e *Equity) Multiplier() int           { return 1 }
func (e *Equity) GetDelta() decimal.Decimal { return decimal.One }

func (e *Equity) GetBid() decimal.Decimal {
	price, _ := e.Book.Bids.NBBO()
	return price
}

func (e *Equity) GetAsk() decimal.Decimal {
	price, _ := e.Book.Asks.NBBO()
	return price
}

func (e *Equity) GetBidSize() uint32 {
	_, size := e.Book.Bids.NBBO()
	return uint32(size.Int())
}

func (e *Equity) GetAskSize() uint32 {
	_, size := e.Book.Asks.NBBO()
	return uint32(size.Int())
}

func (e *Equity) MidPrice() decimal.Decimal {
	return e.GetBid().Add(e.GetAsk()).Half()
}

func (e *Equity) IntrinsicValue(underlyingPrice decimal.Decimal) decimal.Decimal {
	return underlyingPrice
}

func (e *Equity) Ticks() (decimal.Decimal, decimal.Decimal) {
	return cboe.Tick01, cboe.Tick01 // even brk.a ticks at penny increments
}
