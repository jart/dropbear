package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/symbol"
)

// defaultSymbols is the portfolio used for live trading.
// Live 2026-04-28: net=$126, 477 fills, 39900 shares.
var defaultSymbols = []SymbolEntry{
	// NASDAQ longs (~$102k notional at max position)
	{symbol.INTC, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.PYPL, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.CMCSA, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.SOFI, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},

	// NASDAQ shorts (~$101k notional at max position)
	{symbol.HOOD, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.DKNG, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.RIVN, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-500"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.AAL, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-700"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},

	// ARCA (pro-rata)
	{symbol.XLE, Config{
		venue:  alpaca.OrderDestinationARCA,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.FXI, Config{
		venue:  alpaca.OrderDestinationARCA,
		target: decimal.Parse("-500"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},

	// NYSE (pro-rata)
	{symbol.VZ, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.NKE, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("-400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
}
