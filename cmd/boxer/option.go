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

// MarketPrice returns the mid price of the option, or zero if no quote is available.
// This might not be a legal price for an order or quote, e.g. SPX ticks at 0.10 or 0.05 increments.
func (o *Option) MarketPrice() decimal.Decimal {
	return o.Bid.Add(o.Ask).DivInt(2)
}

// String returns a human friendly string representation of the option, e.g. "SPXW 4000.00 C 2024-06-21".
func (o *Option) String() string {
	return fmt.Sprintf("%s %s %s %d-%02d-%02d", o.Sym, o.Strike, o.Class, o.Year, o.Month, o.Day)
}

// OSI returns the OSI code for the option, e.g. "SPXW240621C04000000".
func (o *Option) OSI() string {
	return osi.Encode(o.Sym, o.Strike, byte(o.Class), o.Year, o.Month, o.Day)
}

// IsFresh returns true if option has received a quote recently.
func (o *Option) IsFresh(now clocky.Time) bool {
	return now.Sub(o.TS) <= *freshFlag &&
		es.Price.Sub(o.ES).Abs().Cmp(decimal.One) <= 0
}

// CanBuy returns true if we can buy this option, i.e. it won't close an existing short position.
func (o *Option) CanBuy() bool {
	return !holdings[o.OSI()].IsNegative() &&
		!restrictedToSelling.Contains(o.ID)
}

// CanSell returns true if we can sell this option, i.e. it won't close an existing long position.
func (o *Option) CanSell() bool {
	return !holdings[o.OSI()].IsPositive() &&
		!restrictedToBuying.Contains(o.ID)
}

func compareOptionByStrike(a, b *Option) int {
	return a.Strike.Cmp(b.Strike)
}
