package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/symbol"
)

// boringSymbols is a hedged portfolio of boring, mean-reverting blue chips.
// Use with -boring flag.
var boringSymbols = []SymbolEntry{

	//
	// NASDAQ longs (~$99k max notional)
	//

	// Intel (~$84/share)
	{symbol.INTC, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	// PayPal (~$49/share)
	{symbol.PYPL, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	// Comcast (~$28/share)
	{symbol.CMCSA, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	// Keurig Dr Pepper (~$29/share)
	{symbol.KDP, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},

	//
	// NASDAQ shorts (~$97k max notional)
	//

	// CRISCO (~$87/share)
	{symbol.CSCO, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	// MONDELEZ INTERNATIONAL INC (~$59/share)
	{symbol.MDLZ, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	// Cognizant Technology Solutions Corp (~$55/share)
	{symbol.CTSH, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	// Kraft Heinz Co (~$22/share)
	{symbol.KHC, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},

	//
	// NYSE longs (~$53k max notional)
	//

	// Verizon (~$47/share)
	{symbol.VZ, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	// Coca-Cola Co (~$78/share)
	{symbol.KO, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("100"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},

	//
	// NYSE shorts (~$46k max notional)
	//

	// Dow Inc (~$38/share)
	{symbol.DOW, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("-300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	// Bristol-Myers Squibb Co (~$58/share)
	{symbol.BMY, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("-200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
}
