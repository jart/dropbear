package main

import (
	"dropbear/decimal"
	"dropbear/options"
)

const (
	kStrategyBoxArbitrage = "box arbitrage"
)

var kStrategies = []string{
	kStrategyBoxArbitrage,
}

var kStrategyDefault = map[string]bool{
	kStrategyBoxArbitrage: true,
}

var kStrategyDefaultEOD = map[string]bool{}

func (t *Trader) arbitrageBoxes() {
	price := t.Chain.Price
	if price.IsZero() {
		return
	}

	// Only scan strikes within ±15% of current price for efficiency
	window := price.Mul(decimal.Parse("0.15"))
	lo, hi := price.Sub(window), price.Add(window)

	var strikes []*options.Strike
	for it := t.Chain.Strikes.Iterator(); it.Next(); {
		s := it.Value()
		if s.Price.Cmp(lo) >= 0 && s.Price.Cmp(hi) <= 0 {
			strikes = append(strikes, s)
		}
	}

	var bestLegs []*Leg
	bestNetValue := decimal.Zero
	threshold := t.Config.WeightRisk
	if threshold.IsZero() {
		threshold = decimal.FromInt(5)
	}

	for i := 0; i < len(strikes); i++ {
		for j := i + 1; j < len(strikes); j++ {
			s1, s2 := strikes[i], strikes[j]

			legs := t.getBoxLegs(s1, s2)
			if legs == nil {
				continue
			}

			price := t.choosePriceForOrder(legs)
			if price.IsZero() {
				continue
			}

			payoff := s2.Price.Sub(s1.Price).MulInt(kMultiplier)
			netValue := payoff.Add(price.MulInt(kMultiplier))

			if netValue.Cmp(threshold) > 0 && netValue.Cmp(bestNetValue) > 0 {
				bestNetValue = netValue
				bestLegs = legs
			}
		}
	}

	if bestLegs != nil {
		t.submitOrder(kStrategyBoxArbitrage, bestLegs)
	}
}

func (t *Trader) getBoxLegs(s1, s2 *options.Strike) []*Leg {
	// We need Strike 1 to allow Synthetic Long: Call ModeLong, Put ModeShort
	// We need Strike 2 to allow Synthetic Short: Call ModeShort, Put ModeLong
	
	if s1.Call.Mode == options.ModeLong && s1.Put.Mode == options.ModeShort &&
		s2.Call.Mode == options.ModeShort && s2.Put.Mode == options.ModeLong {
		return []*Leg{
			{Option: s1.Call, Quantity: decimal.One},      // Buy Call s1
			{Option: s1.Put, Quantity: decimal.One.Neg()}, // Sell Put s1
			{Option: s2.Call, Quantity: decimal.One.Neg()}, // Sell Call s2
			{Option: s2.Put, Quantity: decimal.One},       // Buy Put s2
		}
	}

	// Reverse Box?
	// We need Strike 1 to allow Synthetic Short: Call ModeShort, Put ModeLong
	// We need Strike 2 to allow Synthetic Long: Call ModeLong, Put ModeShort
	if s1.Call.Mode == options.ModeShort && s1.Put.Mode == options.ModeLong &&
		s2.Call.Mode == options.ModeLong && s2.Put.Mode == options.ModeShort {
		// This is a Short Box (Selling Strike 1 - Strike 2 spread)
		// Payoff is -(s2 - s1) but we receive a credit.
		// Actually, let's keep it simple and only do Long Boxes for now to avoid confusion.
	}

	return nil
}
