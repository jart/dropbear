package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/cme"
	"dropbear/ds/symbol"
	"fmt"
)

type Future struct {
	ID      uint32          // instrument id
	Symbol  symbol.Symbol   // future symbol, e.g. ES
	Year    int             // future expiration year
	Month   clocky.Month    // future expiration month
	Price   decimal.Decimal // current market midpoint or zero if undefined
	Bid     decimal.Decimal // current best bid price or zero if undefined
	Ask     decimal.Decimal // current best ask price or zero if undefined
	TS      clocky.Time     // timestamp indicating freshness of price data
	BidSize uint32          // number of contracts available at the best bid price
	AskSize uint32          // number of contracts available at the best ask price
}

// MarketPrice returns the mid price of the future, or zero if no quote is available.
// This might not be a legal price for an order or quote (e.g. ES ticks at 0.25 increments, SR1 ticks at 0.005 increments).
func (f *Future) MarketPrice() decimal.Decimal {
	return f.Bid.Add(f.Ask).DivInt(2)
}

// String returns a human friendly string representation of the future, e.g. "ES 2024-06".
func (f *Future) String() string {
	return fmt.Sprintf("%s %d-%02d", f.Symbol, f.Year, f.Month)
}

// CME returns the CME code for the future, e.g. "ESZ6".
func (f *Future) CME() string {
	return fmt.Sprintf("%s%c%d", f.Symbol, cme.MonthCodes[f.Month], f.Year%10)
}
