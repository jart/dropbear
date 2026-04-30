package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/symbol"
)

var kPicks3 = []SymbolEntry{

	{symbol.INTC, Config{
		venue:  alpaca.OrderDestinationARCA,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.06"),
		drift:  decimal.Parse("0.03"),
		greed:  decimal.Parse("0.05"),
	}},

	{symbol.BBIO, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.07"),
		drift:  decimal.Parse("0.04"),
		greed:  decimal.Parse("0.07"),
	}},

	{symbol.IREN, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.06"),
		drift:  decimal.Parse("0.03"),
		greed:  decimal.Parse("0.07"),
	}},

	{symbol.AAL, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.03"),
		drift:  decimal.Parse("0.01"),
		greed:  decimal.Parse("0.03"),
	}},

	{symbol.CORZ, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.05"),
		drift:  decimal.Parse("0.02"),
		greed:  decimal.Parse("0.04"),
	}},

	{symbol.SLV, Config{
		venue:  alpaca.OrderDestinationARCA,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.05"),
		drift:  decimal.Parse("0.03"),
		greed:  decimal.Parse("0.05"),
	}},
}
