package main

import (
	"dropbear/broker/databento"
	"dropbear/decimal"
)

func dbnPrice(p int64) decimal.Decimal {
	if p == databento.UndefPrice {
		return decimal.Zero
	}
	return decimal.Decimal(p / 1000)
}
