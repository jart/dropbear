package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

type Config struct {
	Straddles  decimal.Decimal
	Quantum    decimal.Decimal
	Spread     decimal.Decimal
	Think      clocky.Duration
	StartOfDay int
}
