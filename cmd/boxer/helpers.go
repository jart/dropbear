package main

import (
	"dropbear/broker/databento"
	"dropbear/decimal"
)

var (
	tick05 = decimal.Parse("0.05")
	tick10 = decimal.Parse("0.10")
	three  = decimal.FromInt(3)
)

// optionTick returns the minimum tick size for a Penny Pilot option.
// Options priced under $3 tick in $0.05; $3 and over tick in $0.10.
func optionTick(price decimal.Decimal) decimal.Decimal {
	if price.Cmp(three) < 0 {
		return tick05
	}
	return tick10
}

func dbnPrice(p int64) decimal.Decimal {
	if p == databento.UndefPrice {
		return decimal.Zero
	}
	return decimal.Decimal(p / 1000)
}
