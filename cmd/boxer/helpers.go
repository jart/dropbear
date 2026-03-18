package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
)

var (
	tick05  = decimal.Parse("0.05")
	tick10  = decimal.Parse("0.10")
	quarter = decimal.Parse("0.25")
	three   = decimal.FromInt(3)
	fifteen = decimal.FromInt(15)
)

// optionTick returns the minimum tick size for a Penny Pilot option.
// Options priced under $3 tick in $0.05; $3 and over tick in $0.10.
func optionTick(price decimal.Decimal) decimal.Decimal {
	if price.Cmp(three) < 0 {
		return tick05
	}
	return tick10
}

// isFresh returns true if the option's quote is recent and the underlying
// hasn't moved significantly since the quote was received.
func isFresh(opt *Option, esMid decimal.Decimal) bool {
	if opt.TS.IsZero() {
		return false
	}
	return clocky.Since(opt.TS) <= 150*clocky.Millisecond &&
		esMid.Sub(opt.ES).Abs().Cmp(quarter) <= 0
}

// staleBuffer returns an additional amount to cross through the NBBO on
// stale legs, to survive another arb bot picking off the top of book first.
// It's floor(price / 15) * $0.10, e.g. $0.20 for a $30 option.
func staleBuffer(price decimal.Decimal) decimal.Decimal {
	return price.Abs().Div(fifteen).Truncate().Mul(tick10)
}

// legPrice returns the limit price for a leg based on quote freshness.
// Fresh legs cross the spread (don't rely on PFOF midpoint fills).
// Stale legs cross the spread plus a buffer (pick off stale quotes via Reg NMS).
func legPrice(opt *Option, buying bool, esMid decimal.Decimal) decimal.Decimal {
	if isFresh(opt, esMid) {
		if buying {
			return opt.Ask
		}
		return opt.Bid
	}
	// stale: cross the spread plus buffer to snag next price level
	mid := opt.Bid.Add(opt.Ask).DivInt(2)
	buf := staleBuffer(mid)
	if buying {
		return opt.Ask.Add(buf)
	}
	return opt.Bid.Sub(buf)
}

func dbnPrice(p int64) decimal.Decimal {
	if p == databento.UndefPrice {
		return decimal.Zero
	}
	return decimal.Decimal(p / 1000)
}
