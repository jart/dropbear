package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

type Config struct {
	Straddles    decimal.Decimal
	Quantum      decimal.Decimal
	Spread       decimal.Decimal
	Think        clocky.Duration
	StartOfDay   int
	MaxPending   int
	BypassRisk   bool
	BypassPayoff bool
	BypassScore  bool
	AllowClosing bool
	NoHurry      bool
}
