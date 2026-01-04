package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/ds"
)

type Order struct {
	OrderID        string
	ClientOrderID  string
	Equity         *Equity
	Side           ds.Side
	Status         alpaca.OrderStatus
	Quantity       decimal.Decimal
	LimitPrice     decimal.Decimal
	FilledPrice    decimal.Decimal
	FilledQuantity decimal.Decimal
	TotalFees      decimal.Decimal
}

func (o *Order) Wait() {
	// todo: limit order support
}
