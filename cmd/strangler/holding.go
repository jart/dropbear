package main

import (
	"dropbear/decimal"
	"dropbear/options"
)

type Holding struct {
	Security    options.Security // never nil
	Quantity    decimal.Decimal  // never zero
	AverageCost decimal.Decimal  // weighted average cost of position (always positive; per contract)
}

func (h *Holding) String() string {
	return h.Quantity.String() + " of " + h.Security.String()
}

func (h *Holding) check() {
	if h.Security == nil {
		panic("nil security")
	}
	if h.Quantity.IsZero() {
		panic("zero quantity")
	}
	if !h.AverageCost.IsPositive() {
		panic("non-positive average cost")
	}
}
