package options

import (
	"dropbear/broker/databento"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"math"

	"dropbear/black76"
	"dropbear/osi"
	"dropbear/symbol"
	"fmt"
)

type Option struct {
	Bid     decimal.Decimal           // bid price, e.g. 0.10 (or zero if undefined)
	Ask     decimal.Decimal           // ask price, e.g. 0.15 (or zero if undefined)
	BidSize uint32                    // number of contracts available at the best bid price
	AskSize uint32                    // number of contracts available at the best ask price
	Strike  *Strike                   // option strike price
	ID      uint32                    // instrument id
	Class   databento.InstrumentClass // option class, e.g. 'C' for call, 'P' for put
	Got     Got                       // ready steady go
	Mode    Mode                      // restriction
	Symbol  symbol.Symbol             // underlying symbol, e.g. SPXW, SPY
	Year    int                       // option expiration year
	Month   clocky.Month              // option expiration month
	Day     int                       // option expiration day
	Chain   *Options                  // parent options chain
	TS      clocky.Time               // timestamp of when Bid / Ask was last updated
	IV      decimal.Decimal           // implied volatility at last tick
	Delta   decimal.Decimal           // Black-76 delta at last tick (dPrice/dES)
	Gamma   decimal.Decimal           // Black-76 gamma at last tick (d²Price/dES²)
	Theta   decimal.Decimal           // Black-76 theta at last tick (dPrice/dES)
	Vega    decimal.Decimal           // Black-76 vega at last tick (dPrice/dES)
	mid     decimal.Decimal           // market price when last tick was received
	fut     decimal.Decimal           // es price when last tick was received
	exp     clocky.Time               // caches expiration for quick access
	osi     string                    // caches OSI code for quick access
}

func (o *Option) GetName() string           { return o.OSI() }
func (o *Option) GetID() uint32             { return o.ID }
func (o *Option) GetBid() decimal.Decimal   { return o.Bid }
func (o *Option) GetAsk() decimal.Decimal   { return o.Ask }
func (o *Option) GetBidSize() uint32        { return o.BidSize }
func (o *Option) GetAskSize() uint32        { return o.AskSize }
func (o *Option) Multiplier() int           { return 100 }
func (o *Option) GetDelta() decimal.Decimal { return o.Delta }

// HasQuotes returns true if the option has valid bid and ask prices.
func (o *Option) HasQuotes() bool {
	return (o.Got & (GotBid | GotAsk)) == (GotBid | GotAsk)
}

// HasGreeks returns true if the option has valid IV, Delta, Gamma, Theta, Vega values.
// Having greeks normally implies that this option will also have valid bid and ask prices.
func (o *Option) HasGreeks() bool {
	return (o.Got & GotGreeks) == GotGreeks
}

// MidPrice returns the mid price of the option, or zero if no quote is available.
// This might not be a legal price for an order or quote, e.g. SPX ticks at 0.10 or 0.05 increments.
func (o *Option) MidPrice() decimal.Decimal {
	return o.Bid.Add(o.Ask).DivInt(2)
}

// GetSymbol returns the underlying symbol of the option.
func (o *Option) GetSymbol() symbol.Symbol {
	return o.Symbol
}

// Expiry returns the expiration time of the option.
func (o *Option) Expiry() clocky.Time {
	if o.exp != 0 {
		return o.exp
	}
	o.exp = cboe.GetCloseTime(o.Year, o.Month, o.Day)
	return o.exp
}

// IntrinsicValue returns the intrinsic value of the option contract at the given underlying price.
func (c *Option) IntrinsicValue(underlyingPrice decimal.Decimal) decimal.Decimal {
	if !underlyingPrice.IsPositive() {
		panic("intrinsic value requires positive underlying price")
	}
	if c.Class == 'C' {
		return underlyingPrice.Sub(c.Strike.Price).Max(decimal.Zero)
	}
	return c.Strike.Price.Sub(underlyingPrice).Max(decimal.Zero)
}

// String returns a human friendly string representation of the option, e.g. "SPXW 4000.00 C 2024-06-21".
func (o *Option) String() string {
	return fmt.Sprintf("%s %s %4s %d-%02d-%02d", o.Symbol, o.Strike, o.Class, o.Year, o.Month, o.Day)
}

// StringAligned returns a human friendly string representation of the option with aligned fields for better readability in tables, e.g. "SPXW 4000.00 C 2024-06-21".
func (o *Option) StringAligned() string {
	return fmt.Sprintf("%6s %-8s %-4s %d-%02d-%02d", o.Symbol, o.Strike, o.Class, o.Year, o.Month, o.Day)
}

// OSI returns the OSI code for the option, e.g. "SPXW240621C04000000".
func (o *Option) OSI() string {
	if o.osi != "" {
		return o.osi
	}
	o.osi = osi.Encode(o.Symbol, o.Strike.Price, byte(o.Class), o.Year, o.Month, o.Day)
	return o.osi
}

// ComputeGreeks recomputes the Black-76 delta from the current market mid and SPX price.
// Called on each fresh option tick after Bid/Ask/SPX are updated.
func (o *Option) ComputeGreeks(underlyingPrice, riskFreeRate, futuresPrice decimal.Decimal) {
	if !o.HasQuotes() || !underlyingPrice.IsPositive() {
		o.Got &^= GotGreeks
		return
	}
	o.fut = futuresPrice
	o.mid = o.MidPrice()
	F := underlyingPrice.Float64()
	K := o.Strike.Price.Float64()
	r := riskFreeRate.Float64()
	E := o.Expiry().Sub(o.TS)
	if E <= 0 {
		o.Got &^= GotGreeks
		return
	}
	iv := black76.IV(F, K, r, E, o.mid.Float64(), o.Class == databento.InstrumentClassCall)
	if iv <= 0 || math.IsNaN(iv) || math.IsInf(iv, 0) {
		o.Got &^= GotGreeks
		return
	}
	var delta, theta, gamma, vega float64
	if o.Class == databento.InstrumentClassCall {
		delta, theta, gamma, vega = black76.CallGreeks(F, K, r, E, iv)
	} else {
		delta, theta, gamma, vega = black76.PutGreeks(F, K, r, E, iv)
	}
	o.Delta = decimal.FromFloat64(delta)
	o.Theta = decimal.FromFloat64(theta)
	o.Gamma = decimal.FromFloat64(gamma)
	o.Vega = decimal.FromFloat64(vega / 100)
	o.IV = decimal.FromFloat64(iv)
	o.Got |= GotGreeks
}
