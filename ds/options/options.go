package options

import (
	"dropbear/broker/databento"
	"dropbear/decimal"

	"github.com/emirpasic/gods/v2/maps/treemap"
)

// Options represents the current state of an option chain.
type Options struct {
	Price          decimal.Decimal
	AtTheMoney     *Strike
	Strikes        *treemap.Map[decimal.Decimal, *Strike]
	pendingStrikes *treemap.Map[decimal.Decimal, *Strike]
}

// NewOptions creates a new Options instance with an initialized strikes map.
func NewOptions() *Options {
	return &Options{
		Strikes:        treemap.New[decimal.Decimal, *Strike](),
		pendingStrikes: treemap.New[decimal.Decimal, *Strike](),
	}
}

// Add adds an option to the option chain.
// This should be called repeatedly each time its quotes are updated.
// Returns true if this operation resulted in Price/AtTheMoney being updated.
func (oc *Options) Add(o *Option) bool {
	strikePrice := o.Strike.Price
	strike, _ := oc.Strikes.Get(strikePrice)
	if strike == nil {
		strike = oc.populateStrike(o, strikePrice)
	}
	if o.HasQuotes() && strike.IsReady() {
		if oc.AtTheMoney == nil && strike.UnderlyingPrice().IsPositive() {
			oc.AtTheMoney = strike
		}
		if strike == oc.AtTheMoney {
			return oc.updateFields()
		}
	}
	return false
}

//go:noinline
func (oc *Options) updateFields() bool {
	newPrice := oc.AtTheMoney.UnderlyingPrice()
	if !newPrice.IsPositive() {
		return false
	}
	var atm *Strike
	_, strike1, _ := oc.Strikes.Floor(newPrice)
	_, strike2, _ := oc.Strikes.Ceiling(newPrice)
	if strike1 != nil && strike2 != nil {
		distance1 := strike1.Price.Sub(newPrice).Abs()
		distance2 := strike2.Price.Sub(newPrice).Abs()
		if distance2.Cmp(distance1) < 0 {
			atm = strike2
		} else {
			atm = strike1
		}
	} else if strike2 != nil {
		atm = strike2
	} else {
		atm = strike1
	}
	if atm != oc.AtTheMoney {
		oc.AtTheMoney = atm
		newPrice = atm.UnderlyingPrice()
	}
	if !newPrice.IsPositive() || newPrice == oc.Price {
		return false
	}
	oc.Price = newPrice
	return true
}

//go:noinline
func (oc *Options) populateStrike(o *Option, strikePrice decimal.Decimal) *Strike {
	strike, _ := oc.pendingStrikes.Get(strikePrice)
	if strike == nil {
		strike = &Strike{Price: strikePrice}
		oc.pendingStrikes.Put(strikePrice, strike)
	}
	o.Strike = strike
	if o.Class == databento.InstrumentClassCall {
		strike.Call = o
	} else {
		strike.Put = o
	}
	if strike.IsReady() {
		oc.pendingStrikes.Remove(strikePrice)
		_, strike.Prev, _ = oc.Strikes.Floor(strikePrice)
		_, strike.Next, _ = oc.Strikes.Ceiling(strikePrice)
		oc.Strikes.Put(strikePrice, strike)
		if strike.Prev != nil {
			strike.Prev.Next = strike
		}
		if strike.Next != nil {
			strike.Next.Prev = strike
		}
	}
	return strike
}
