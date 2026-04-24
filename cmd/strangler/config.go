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
	Tolerance  decimal.Decimal
	Contracts  decimal.Decimal
	Spread     decimal.Decimal
	Wing       decimal.Decimal
	Risk       decimal.Decimal
	Lookback   clocky.Duration
	Patience   clocky.Duration
	Chase      clocky.Duration
	Age        clocky.Duration
	Samples    int
	Strikes    int
	StartOfDay int
	Schwab     bool
	DMA        alpaca.OrderDestination
	Listen     string
}
