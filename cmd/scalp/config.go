package main

import (
	"dropbear/decimal"
	"dropbear/symbol"
)

type Config struct {
	Symbol     symbol.Symbol
	Direction  decimal.Decimal
	Straddles  decimal.Decimal
	Quantum    decimal.Decimal
	Spread     decimal.Decimal
	Strikes    int
	StartOfDay int
}
