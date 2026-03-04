package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
	"fmt"
)

type Future struct {
	ID    uint32          // instrument id
	Sym   symbol.Symbol   // future symbol, e.g. ES
	Year  int             // future expiration year
	Month clocky.Month    // future expiration month
	Price decimal.Decimal // current market midpoint or zero if undefined
	Bid   decimal.Decimal // current best bid price or zero if undefined
	Ask   decimal.Decimal // current best ask price or zero if undefined
}

func (f *Future) String() string {
	return fmt.Sprintf("%s %d-%02d", f.Sym, f.Year, f.Month)
}

func (f *Future) CME() string {
	return fmt.Sprintf("%s%c%d", f.Sym, CMEMonthCodes[f.Month], f.Year%10)
}
