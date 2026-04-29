package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/symbol"
)

var kPicks3 = []SymbolEntry{
	{symbol.INTC, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.03"),
		greed:  decimal.Parse("0.05"),
	}},
	{symbol.CSCO, Config{
		venue:  alpaca.OrderDestinationNone,
		target: decimal.Parse("-300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
		greed:  decimal.Parse("0.05"),
	}},
	{symbol.IREN, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.03"),
		drift:  decimal.Parse("0.03"),
		greed:  decimal.Parse("0.03"),
	}},
	{symbol.KO, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.00"),
		drift:  decimal.Parse("0.01"),
		greed:  decimal.Parse("0.03"),
	}},
	{symbol.AAL, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.01"),
		drift:  decimal.Parse("0.01"),
		greed:  decimal.Parse("0.03"),
	}},
	{symbol.VISN, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.01"),
		greed:  decimal.Parse("0.03"),
	}},
	{symbol.CORZ, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.03"),
		drift:  decimal.Parse("0.01"),
		greed:  decimal.Parse("0.04"),
	}},
	{symbol.MARA, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.01"),
		greed:  decimal.Parse("0.03"),
	}},
	{symbol.WBD, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.01"),
		greed:  decimal.Parse("0.03"),
	}},
	{symbol.BBIO, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.07"),
		drift:  decimal.Parse("0.04"),
		greed:  decimal.Parse("0.07"),
	}},
}
