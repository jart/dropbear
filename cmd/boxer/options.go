package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
	"fmt"
)

type Options struct {
	ID     uint32                    // instrument id
	Class  databento.InstrumentClass // option class, e.g. 'C' for call, 'P' for put
	Sym    symbol.Symbol             // option symbol, e.g. SPXW, SPY
	Strike decimal.Decimal           // option strike price
	Year   int                       // option expiration year
	Month  clocky.Month              // option expiration month
	Day    int                       // option expiration day
	Bid    decimal.Decimal           // bid price, e.g. 0.10 (or zero if undefined)
	Ask    decimal.Decimal           // ask price, e.g. 0.15 (or zero if undefined)
	Qty    int                       // contracts owned (positive for long, negative for short)
}

func (o *Options) String() string {
	return fmt.Sprintf("%s %s %-4s %d-%02d-%02d", o.Sym, o.Strike, o.Class, o.Year, o.Month, o.Day)
}

func (o *Options) OSI() string {
	var buf [21]byte
	// symbol: 6 chars, space-padded
	for i := range buf[:6] {
		buf[i] = ' '
	}
	s := uint64(o.Sym)
	for i := 0; i < 6 && s != 0; i++ {
		buf[i] = byte(s)
		s >>= 8
	}
	// expiration: YYMMDD
	yy := o.Year - 2000
	buf[6] = byte(yy/10) + '0'
	buf[7] = byte(yy%10) + '0'
	buf[8] = byte(o.Month/10) + '0'
	buf[9] = byte(o.Month%10) + '0'
	buf[10] = byte(o.Day/10) + '0'
	buf[11] = byte(o.Day%10) + '0'
	// class
	buf[12] = byte(o.Class)
	// strike × 1000, 8 digits zero-padded
	strike := o.Strike.MulInt(1000).Int64()
	for i := 20; i >= 13; i-- {
		buf[i] = byte(strike%10) + '0'
		strike /= 10
	}
	return string(buf[:])
}
