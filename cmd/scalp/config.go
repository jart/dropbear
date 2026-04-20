package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
)

type Config struct {
	Symbol     symbol.Symbol
	Direction  decimal.Decimal
	Straddles  decimal.Decimal
	Tolerance  decimal.Decimal
	Quantum    decimal.Decimal
	Spread     decimal.Decimal
	Patience   clocky.Duration
	Strikes    int
	StartOfDay int
}
