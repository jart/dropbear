package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/osi"
	"dropbear/ds/symbol"
	"fmt"
)

type Option struct {
	ID     uint32                    // instrument id
	Class  databento.InstrumentClass // option class, e.g. 'C' for call, 'P' for put
	Sym    symbol.Symbol             // option symbol, e.g. SPXW, SPY
	Strike decimal.Decimal           // option strike price
	Year   int                       // option expiration year
	Month  clocky.Month              // option expiration month
	Day    int                       // option expiration day
	Bid    decimal.Decimal           // bid price, e.g. 0.10 (or zero if undefined)
	Ask    decimal.Decimal           // ask price, e.g. 0.15 (or zero if undefined)
	TS     clocky.Time               // timestamp of when Bid / Ask was last updated
	ES     decimal.Decimal           // price of ES futures at time last tick was received
}

func (o *Option) String() string {
	return fmt.Sprintf("%s %s %-4s %d-%02d-%02d", o.Sym, o.Strike, o.Class, o.Year, o.Month, o.Day)
}

func (o *Option) OSI() string {
	return osi.Encode(o.Sym, o.Strike, byte(o.Class), o.Year, o.Month, o.Day)
}

func compareOptionByStrike(a, b *Option) int {
	return a.Strike.Cmp(b.Strike)
}
