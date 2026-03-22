package options

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"math"

	"dropbear/ds/black76"
	"dropbear/ds/osi"
	"dropbear/ds/symbol"
	"fmt"
)

type Option struct {
	ID      uint32                    // instrument id
	Class   databento.InstrumentClass // option class, e.g. 'C' for call, 'P' for put
	Got     Got                       // ready steady go
	Sym     symbol.Symbol             // option symbol, e.g. SPXW, SPY
	Strike  decimal.Decimal           // option strike price
	Year    int                       // option expiration year
	Month   clocky.Month              // option expiration month
	Day     int                       // option expiration day
	Bid     decimal.Decimal           // bid price, e.g. 0.10 (or zero if undefined)
	Ask     decimal.Decimal           // ask price, e.g. 0.15 (or zero if undefined)
	BidSize uint32                    // number of contracts available at the best bid price
	AskSize uint32                    // number of contracts available at the best ask price
	TS      clocky.Time               // timestamp of when Bid / Ask was last updated
	IV      decimal.Decimal           // implied volatility at last tick
	Price   decimal.Decimal           // Black-76 price at last tick
	Delta   decimal.Decimal           // Black-76 delta at last tick (dPrice/dES)
	Gamma   decimal.Decimal           // Black-76 gamma at last tick (d²Price/dES²)
	Theta   decimal.Decimal           // Black-76 theta at last tick (dPrice/dES)
	Vega    decimal.Decimal           // Black-76 theta at last tick (dPrice/dES)
	mid     decimal.Decimal           // market price when last tick was received
	fut     decimal.Decimal           // es price when last tick was received
}

// IsReady returns true if the option has a two-sided quote and greeks.
func (o *Option) IsReady() bool {
	return (o.Got & GotGreeks) == GotGreeks
}

// IntrinsicValue returns the intrinsic value of the option contract at the given underlying price.
func (c *Option) IntrinsicValue(underlyingPrice decimal.Decimal) decimal.Decimal {
	if c.Class == 'C' {
		return underlyingPrice.Sub(c.Strike).Max(decimal.Zero)
	}
	return c.Strike.Sub(underlyingPrice).Max(decimal.Zero)
}

// MarketPrice returns the mid price of the option, or zero if no quote is available.
// This might not be a legal price for an order or quote, e.g. SPX ticks at 0.10 or 0.05 increments.
func (o *Option) MarketPrice() decimal.Decimal {
	return o.Bid.Add(o.Ask).DivInt(2)
}

// FairPrice returns the delta-adjusted mid price using the latest ES futures price.
// This predicts what the option mid should be right now, even if the OPRA quote is stale,
// by applying: fair = lastMid + delta * (currentES - esAtLastQuote).
// Returns the stale market mid if delta hasn't been computed yet.
func (o *Option) FairPrice(futuresPrice decimal.Decimal) decimal.Decimal {
	if !o.IsReady() || futuresPrice.IsZero() || o.fut.IsZero() {
		return o.MarketPrice()
	}
	dES := futuresPrice.Sub(o.fut)
	adj := o.Delta.Mul(dES)
	fair := o.mid.Add(adj)
	if !fair.IsPositive() {
		panic("computed bad fair price")
	}
	return fair
}

// String returns a human friendly string representation of the option, e.g. "SPXW 4000.00 C 2024-06-21".
func (o *Option) String() string {
	return fmt.Sprintf("%s %s %4s %d-%02d-%02d", o.Sym, o.Strike, o.Class, o.Year, o.Month, o.Day)
}

// OSI returns the OSI code for the option, e.g. "SPXW240621C04000000".
func (o *Option) OSI() string {
	return osi.Encode(o.Sym, o.Strike, byte(o.Class), o.Year, o.Month, o.Day)
}

// ComputeGreeks recomputes the Black-76 delta from the current market mid and SPX price.
// Called on each fresh option tick after Bid/Ask/SPX are updated.
func (o *Option) ComputeGreeks(underlyingPrice, riskFreeRate, futuresPrice decimal.Decimal) {
	if (o.Got&(GotBid|GotAsk)) != (GotBid|GotAsk) || !underlyingPrice.IsPositive() {
		o.Got &^= GotGreeks
		return
	}
	o.fut = futuresPrice
	o.mid = o.MarketPrice()
	F := underlyingPrice.Float64()
	K := o.Strike.Float64()
	r := riskFreeRate.Float64()
	T := o.timeToExpiry()
	if T <= 0 {
		o.Got &^= GotGreeks
		return
	}
	iv := black76.IV(F, K, r, T, o.mid.Float64(), o.Class == databento.InstrumentClassCall)
	if iv <= 0 || math.IsNaN(iv) || math.IsInf(iv, 0) {
		o.Got &^= GotGreeks
		return
	}
	var price, delta, theta, gamma, vega float64
	if o.Class == databento.InstrumentClassCall {
		price, delta, theta, gamma, vega = black76.Call(F, K, r, T, iv)
	} else {
		price, delta, theta, gamma, vega = black76.Put(F, K, r, T, iv)
	}
	o.Price = decimal.FromFloat64(price)
	o.Delta = decimal.FromFloat64(delta)
	o.Theta = decimal.FromFloat64(theta)
	o.Gamma = decimal.FromFloat64(gamma)
	o.Vega = decimal.FromFloat64(vega)
	o.IV = decimal.FromFloat64(iv)
	o.Got |= GotGreeks
}

// timeToExpiry returns number of years until expiration.
func (o *Option) timeToExpiry() float64 {
	// TODO(jart): handle early closing days
	expiry := clocky.Date(o.Year, o.Month, o.Day, 16, 0, 0, 0)
	now := clocky.Now()
	if now >= expiry {
		return 0
	}
	return float64(expiry-now) / (365.25 * 24 * 3600 * 1e9)
}
