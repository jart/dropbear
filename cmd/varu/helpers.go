package main

import (
	"dropbear/decimal"
	"dropbear/ds/symbol"
)

const (
	kSPXW = symbol.Symbol('S' | 'P'<<8 | 'X'<<16 | 'W'<<24)
	kNDX  = symbol.Symbol('N' | 'D'<<8 | 'X'<<16)
)

var (
	kThree  = decimal.FromInt(3)
	kTick01 = decimal.Parse("0.01")
	kTick05 = decimal.Parse("0.05")
	kTick10 = decimal.Parse("0.10")
)

// getTicks returns the minimum tick size.
func getTicks() (decimal.Decimal, decimal.Decimal) {
	switch gSymbol {
	case kSPXW, kNDX:
		return kTick05, kTick10
	default:
		return kTick01, kTick05
	}
}

// minTick returns the minimum tick size.
func minTick() decimal.Decimal {
	minTick, _ := getTicks()
	return minTick
}

// incTick increases an spx option's price by one tick.
func incTick(price decimal.Decimal) decimal.Decimal {
	minTick, maxTick := getTicks()
	if price.Abs().Cmp(kThree) < 0 {
		return price.Add(minTick)
	}
	return price.Add(maxTick)
}

// decTick reduces an spx option's price by one tick.
func decTick(price decimal.Decimal) decimal.Decimal {
	minTick, maxTick := getTicks()
	if price.Abs().Cmp(kThree) <= 0 {
		return price.Sub(minTick)
	}
	return price.Sub(maxTick)
}

// quantizeTruncate rounds to the SPX tick size for buying.
func quantizeTruncate(price decimal.Decimal) decimal.Decimal {
	tick := optionTick(price)
	return price.QuantizeTruncate(tick).Max(tick)
}

// quantizeAway rounds to the SPX tick size for selling.
func quantizeAway(price decimal.Decimal) decimal.Decimal {
	tick := optionTick(price)
	return price.QuantizeAway(tick).Max(tick)
}

// optionTick returns the minimum tick size for a Penny Pilot option.
// Options priced under $3 tick in $0.05; $3 and over tick in $0.10.
func optionTick(price decimal.Decimal) decimal.Decimal {
	minTick, maxTick := getTicks()
	if price.Abs().Cmp(kThree) < 0 {
		return minTick
	}
	return maxTick
}
