package main

import (
	"dropbear/decimal"
)

// incTick increases an spx option's price by one tick.
func incTick(price decimal.Decimal) decimal.Decimal {
	switch gSymbol {
	case kSPXW:
		if price.Abs().Cmp(kThree) <= 0 {
			return price.Add(kTick05)
		}
		return price.Add(kTick10)
	default:
		return price.Add(kTick01)
	}
}

// decTick reduces an spx option's price by one tick.
func decTick(price decimal.Decimal) decimal.Decimal {
	switch gSymbol {
	case kSPXW:
		if price.Abs().Cmp(kThree) <= 0 {
			return price.Sub(kTick05)
		}
		return price.Sub(kTick10)
	default:
		return price.Sub(kTick01)
	}
}

// quantizeTruncate rounds to the SPX tick size for buying.
func quantizeTruncate(price decimal.Decimal) decimal.Decimal {
	tick := optionTick(price)
	price = price.QuantizeTruncate(tick)
	if price.IsZero() {
		// handle cases like 0.00/0.05 bid/ask on far otm strikes
		price = tick
	}
	return price
}

// quantizeAway rounds to the SPX tick size for selling.
func quantizeAway(price decimal.Decimal) decimal.Decimal {
	return price.QuantizeAway(optionTick(price))
}

// optionTick returns the minimum tick size for a Penny Pilot option.
// Options priced under $3 tick in $0.05; $3 and over tick in $0.10.
func optionTick(price decimal.Decimal) decimal.Decimal {
	switch gSymbol {
	case kSPXW:
		if price.Abs().Cmp(kThree) < 0 {
			return kTick05
		}
		return kTick10
	default:
		return kTick01
	}
}
