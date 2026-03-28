package main

import (
	"dropbear/decimal"
	"dropbear/ds/options"
)

type Holding struct {
	Quantity    decimal.Decimal // never zero
	Option      *options.Option // never nil
	AverageCost decimal.Decimal // weighted average cost of position (always positive; per contract)
}

func (h *Holding) String() string {
	return h.Quantity.String() + " of " + h.Option.String()
}

func (h *Holding) check() {
	if h.Option == nil {
		panic("nil option")
	}
	if h.Quantity.IsZero() {
		panic("zero quantity")
	}
	if !h.AverageCost.IsPositive() {
		panic("non-positive average cost")
	}
}
