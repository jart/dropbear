package options

import (
	"dropbear/broker/databento"
	"dropbear/decimal"

	"github.com/emirpasic/gods/v2/maps/treemap"
)

// Options represents the current state of an option chain.
type Options struct {
	Price          decimal.Decimal
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

// UpdateOption tracks an option whose quote has changed.
// Returns true if caller should recompute its greeks too.
func (os *Options) UpdateOption(o *Option) bool {
	mustRecomputeGreeks := false
	if (o.Got & (GotBid | GotAsk)) == (GotBid | GotAsk) {
		s, ok := os.Strikes.Get(o.Strike)
		if ok {
			if os.updatePrice(s) {
				mustRecomputeGreeks = true
			}
		} else {
			s, ok = os.pendingStrikes.Get(o.Strike)
			if !ok {
				s = &Strike{}
				os.pendingStrikes.Put(o.Strike, s)
			}
			if o.Class == databento.InstrumentClassCall {
				s.Call = o
			} else {
				s.Put = o
			}
			if s.IsReady() {
				os.pendingStrikes.Remove(o.Strike)
				os.Strikes.Put(o.Strike, s)
				if os.updatePrice(s) {
					mustRecomputeGreeks = true
				}
			}
		}
	}
	return mustRecomputeGreeks
}

// updatePrice estimates the current SPX price based on options quotes.
// We use the closest strike to the money to compute our inferred underlying price.
// You must provide any ready strike, so this can compute an estimated price first.
func (os *Options) updatePrice(strike *Strike) bool {
	changed := false
	estimate := strike.UnderlyingPrice()
	_, ceilStrike, ceilFound := os.Strikes.Ceiling(estimate)
	_, floorStrike, floorFound := os.Strikes.Floor(estimate)
	if ceilFound && floorFound && ceilStrike.IsReady() && floorStrike.IsReady() {
		ceilDistance := ceilStrike.Strike().Sub(estimate).Abs()
		floorDistance := floorStrike.Strike().Sub(estimate).Abs()
		var closestStrike *Strike
		var closestDistance decimal.Decimal
		if ceilDistance.Cmp(floorDistance) <= 0 {
			closestStrike = ceilStrike
			closestDistance = ceilDistance
		} else {
			closestStrike = floorStrike
			closestDistance = floorDistance
		}
		if closestDistance.Cmp(decimal.FromInt(5)) <= 0 {
			spxPrice := closestStrike.UnderlyingPrice()
			changed = spxPrice.Cmp(os.Price) != 0
			os.Price = spxPrice
		}
	}
	return changed
}
