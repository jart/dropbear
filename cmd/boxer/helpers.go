package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
)

var (
	tick05 = decimal.Parse("0.05")
	tick10 = decimal.Parse("0.10")
	three  = decimal.FromInt(3)
)

// optionTick returns the minimum tick size for a Penny Pilot option.
// Options priced under $3 tick in $0.05; $3 and over tick in $0.10.
func optionTick(price decimal.Decimal) decimal.Decimal {
	if price.Cmp(three) < 0 {
		return tick05
	}
	return tick10
}

var quarter = decimal.Parse("0.25")

// isFresh returns true if the option's quote is recent and the underlying
// hasn't moved significantly since the quote was received.
func isFresh(opt *Option, esMid decimal.Decimal) bool {
	if opt.TS.IsZero() {
		return false
	}
	return clocky.Since(opt.TS) <= 150*clocky.Millisecond &&
		esMid.Sub(opt.ES).Abs().Cmp(quarter) <= 0
}

// legPrice returns the limit price for a leg based on quote freshness.
// Fresh legs get midpoint pricing (demand edge from market makers).
// Stale legs cross the spread (pick off stale quotes via Reg NMS).
func legPrice(opt *Option, buying bool, esMid decimal.Decimal) decimal.Decimal {
	if isFresh(opt, esMid) {
		mid := opt.Bid.Add(opt.Ask).DivInt(2)
		t := optionTick(mid)
		if buying {
			return mid.QuantizeTruncate(t)
		}
		return mid.QuantizeAway(t)
	}
	// stale: cross the spread
	if buying {
		return opt.Ask
	}
	return opt.Bid
}

func dbnPrice(p int64) decimal.Decimal {
	if p == databento.UndefPrice {
		return decimal.Zero
	}
	return decimal.Decimal(p / 1000)
}
