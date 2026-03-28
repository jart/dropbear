package main

import (
	"dropbear/decimal"
	"dropbear/ds/options"
	"fmt"
)

type Leg struct {
	Option   *options.Option
	Quantity decimal.Decimal
	Filled   bool
}

func (leg *Leg) String() string {
	return fmt.Sprintf("%s %s", leg.Quantity, leg.Option)
}
