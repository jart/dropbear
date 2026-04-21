package main

import (
	"dropbear/broker/alpaca"
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
	Wing       decimal.Decimal
	Patience   clocky.Duration
	Strikes    int
	StartOfDay int
	Schwab     bool
	DMA        alpaca.OrderDestination
	Listen     string
}
