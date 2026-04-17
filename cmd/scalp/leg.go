package main

import (
	"dropbear/decimal"
	"dropbear/options"
	"fmt"
)

type Leg struct {
	Security options.Security
	Quantity decimal.Decimal
	Filled   bool
}

func (leg *Leg) String() string {
	return fmt.Sprintf("%s %s", leg.Quantity, leg.Security.String())
}
