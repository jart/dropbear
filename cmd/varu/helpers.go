package main

import (
	"dropbear/cboe"
	"dropbear/decimal"
	"dropbear/symbol"
)

// getTicks returns the minimum tick size.
func getTicks(sym symbol.Symbol) (decimal.Decimal, decimal.Decimal) {
	switch sym {
	case symbol.SPXW, symbol.NDX, symbol.RUTW:
		return cboe.Tick05, cboe.Tick10
	default:
		return cboe.Tick01, cboe.Tick05
	}
}

// minTick returns the minimum tick size.
func minTick(sym symbol.Symbol) decimal.Decimal {
	minTick, _ := getTicks(sym)
	return minTick
}

// incTick increases an spx option's price by one tick.
func incTick(sym symbol.Symbol, price decimal.Decimal) decimal.Decimal {
	minTick, maxTick := getTicks(sym)
	if price.Abs().Cmp(decimal.Three) < 0 {
		return price.Add(minTick)
	}
	return price.Add(maxTick)
}

// decTick reduces an spx option's price by one tick.
func decTick(sym symbol.Symbol, price decimal.Decimal) decimal.Decimal {
	minTick, maxTick := getTicks(sym)
	if price.Abs().Cmp(decimal.Three) <= 0 {
		return price.Sub(minTick)
	}
	return price.Sub(maxTick)
}

// quantizeTruncate rounds to the SPX tick size for buying.
func quantizeTruncate(sym symbol.Symbol, price decimal.Decimal) decimal.Decimal {
	tick := optionTick(sym, price)
	return price.QuantizeTruncate(tick).Max(tick)
}

// quantizeAway rounds to the SPX tick size for selling.
func quantizeAway(sym symbol.Symbol, price decimal.Decimal) decimal.Decimal {
	tick := optionTick(sym, price)
	return price.QuantizeAway(tick).Max(tick)
}

// optionTick returns the minimum tick size for a Penny Pilot option.
// Options priced under $3 tick in $0.05; $3 and over tick in $0.10.
func optionTick(sym symbol.Symbol, price decimal.Decimal) decimal.Decimal {
	minTick, maxTick := getTicks(sym)
	if price.Abs().Cmp(decimal.Three) < 0 {
		return minTick
	}
	return maxTick
}

// boolToString converts a boolean to "true" or "false".
func boolToString(b bool) string {
	if b {
		return "true"
	} else {
		return "false"
	}
}
