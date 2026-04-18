package options

import (
	"dropbear/decimal"
	"dropbear/symbol"
)

type Security interface {
	String() string
	GetID() uint32
	Name() string
	GetBid() decimal.Decimal
	GetAsk() decimal.Decimal
	GetSymbol() symbol.Symbol
	GetBidSize() uint32
	GetAskSize() uint32
	MidPrice() decimal.Decimal
	Multiplier() int
	IntrinsicValue(decimal.Decimal) decimal.Decimal
	GetDelta() decimal.Decimal
	Ticks() (decimal.Decimal, decimal.Decimal)
}
