package main

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
	Mid    decimal.Decimal           // market mid at last tick (cached for delta prediction)
	IV     float64                   // implied volatility at last tick
	Delta  float64                   // Black-76 delta at last tick (dPrice/dES)
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
func (o *Option) FairPrice() decimal.Decimal {
	if o.Delta == 0 || o.Mid.IsZero() || es == nil {
		return o.MarketPrice()
	}
	dES := es.Price.Sub(o.ES)
	adj := decimal.FromFloat64(o.Delta * dES.Float64())
	fair := o.Mid.Add(adj)
	if fair.IsNegative() {
		return decimal.Zero
	}
	return fair
}

// String returns a human friendly string representation of the option, e.g. "SPXW 4000.00 C 2024-06-21".
func (o *Option) String() string {
	return fmt.Sprintf("%s %s %s %d-%02d-%02d", o.Sym, o.Strike, o.Class, o.Year, o.Month, o.Day)
}

// OSI returns the OSI code for the option, e.g. "SPXW240621C04000000".
func (o *Option) OSI() string {
	return osi.Encode(o.Sym, o.Strike, byte(o.Class), o.Year, o.Month, o.Day)
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

// UpdateDelta recomputes the Black-76 delta from the current market mid and ES price.
// Called on each fresh option tick after Bid/Ask/ES are updated.
func (o *Option) UpdateDelta() {
	mid := o.MarketPrice()
	if mid.IsZero() || o.ES.IsZero() {
		return
	}
	o.Mid = mid
	F := o.ES.Float64()
	K := o.Strike.Float64()
	T := o.timeToExpiry()
	if T <= 0 {
		return
	}
	r := riskFreeRate()
	iv := black76.IV(F, K, r, T, mid.Float64(), o.Class == databento.InstrumentClassCall)
	if iv <= 0 || math.IsNaN(iv) {
		return
	}
	o.IV = iv
	if o.Class == databento.InstrumentClassCall {
		o.Delta = black76.CallDelta(F, K, r, T, iv)
	} else {
		o.Delta = black76.PutDelta(F, K, r, T, iv)
	}
}

// Gamme returns the Black-76 gamma of the option (d²Price/dES²).
func (o *Option) Gamma() decimal.Decimal {
	F, K, r, T, iv, ok := o.greekParams()
	if !ok {
		return decimal.Zero
	}
	return decimal.FromFloat64(black76.Gamma(F, K, r, T, iv))
}

// Theta returns the daily Black-76 theta of the option (dPrice/dT per day).
func (o *Option) Theta() decimal.Decimal {
	F, K, r, T, iv, ok := o.greekParams()
	if !ok {
		return decimal.Zero
	}
	if o.Class == databento.InstrumentClassCall {
		return decimal.FromFloat64(black76.CallTheta(F, K, r, T, iv))
	}
	return decimal.FromFloat64(black76.PutTheta(F, K, r, T, iv))
}

// Vega returns the Black-76 vega of the option (dPrice/dIV).
func (o *Option) Vega() decimal.Decimal {
	F, K, r, T, iv, ok := o.greekParams()
	if !ok {
		return decimal.Zero
	}
	return decimal.FromFloat64(black76.Vega(F, K, r, T, iv))
}

// greekParams returns the parameters needed to compute Black-76 greeks, or ok=false if any are invalid.
func (o *Option) greekParams() (F, K, r, T, iv float64, ok bool) {
	F = o.ES.Float64()
	K = o.Strike.Float64()
	T = o.timeToExpiry()
	if T <= 0 || o.IV <= 0 || F <= 0 {
		return
	}
	return F, K, riskFreeRate(), T, o.IV, true
}

// timeToExpiry returns years until 4:00 PM ET on the option's expiration date.
func (o *Option) timeToExpiry() float64 {
	expiry := clocky.Date(o.Year, o.Month, o.Day, 16, 0, 0, 0)
	now := clocky.Now()
	if now >= expiry {
		return 0
	}
	return float64(expiry-now) / (365.25 * 24 * 3600 * 1e9)
}
