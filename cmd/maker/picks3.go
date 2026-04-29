package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/symbol"
)

var kPicks3 = []SymbolEntry{
	{symbol.GOOG, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("160"),
		qty:    decimal.Parse("40"),
		spread: decimal.Parse("0.05"),
		drift:  decimal.Parse("0.05"),
		greed:  decimal.Parse("0.08"),
	}},
}
