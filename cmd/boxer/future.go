package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
	"fmt"
)

type Future struct {
	ID     uint32          // instrument id
	Symbol symbol.Symbol   // future symbol, e.g. ES
	Year   int             // future expiration year
	Month  clocky.Month    // future expiration month
	Price  decimal.Decimal // current market midpoint or zero if undefined
	Bid    decimal.Decimal // current best bid price or zero if undefined
	Ask    decimal.Decimal // current best ask price or zero if undefined
	TS     clocky.Time     // timestamp of last tick
}

func (f *Future) String() string {
	return fmt.Sprintf("%s %d-%02d", f.Symbol, f.Year, f.Month)
}

func (f *Future) CME() string {
	return fmt.Sprintf("%s%c%d", f.Symbol, CMEMonthCodes[f.Month], f.Year%10)
}
