package main

import (
	"dropbear/decimal"
)

// updateSPXPrice estimates the current SPX price based on options quotes.
// We use the closest strike to the money to compute our inferred underlying price.
// You must provide any ready strike, so this can compute an estimated price first.
func updateSPXPrice(strike *Strike) bool {
	changed := false
	estimate := strike.UnderlyingPrice()
	_, ceilStrike, ceilFound := gStrikes.Ceiling(estimate)
	_, floorStrike, floorFound := gStrikes.Floor(estimate)
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
			changed = spxPrice.Cmp(gSPXPrice) != 0
			gSPXPrice = spxPrice
		}
	}
	return changed
}
