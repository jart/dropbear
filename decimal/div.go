package decimal

import (
	"math"
	"math/bits"
)

// Div divides d by o.
// This uses Bankers' Rounding.
// Panics on overflow (MinInt64 / -1) or division by zero.
func (d Decimal) Div(o Decimal) Decimal {

	// get absolute values and sign
	di := int64(d)
	oi := int64(o)
	dm := di >> 63
	ud := uint64((di ^ dm) - dm)
	om := oi >> 63
	uo := uint64((oi ^ om) - om)
	sm := (di ^ oi) >> 63

	// 128-bit Mul: (hi, lo) = ud * Scale
	// Since ud <= 2^63 and Scale = 1e8, the product fits in ~90 bits.
	// hi = (ud * Scale) >> 64 <= (2^63 * 1e8) >> 64 = 5e7 < 2^63.
	hi, lo := bits.Mul64(ud, uint64(Scale))

	// division: quo = result, rem = remainder
	// Note: bits.Div64 panics if hi >= uo, which happens when dividing
	// by very small numbers (e.g., Max / Epsilon). That's the desired behavior.
	quo, rem := bits.Div64(hi, lo, uo)

	// Banker's Rounding (Round Half To Even)
	// We want to increment quo if:
	// 1. rem > uo/2
	// 2. rem == uo/2 AND quo is odd AND uo is even (exact half case)
	//
	// This is mathematically equivalent to:
	// (2 * rem) + (quo & 1) > uo
	//
	// Proof:
	// - If rem > uo/2: 2*rem > uo. (2*rem + bit) > uo holds.
	// - If rem < uo/2: 2*rem < uo. Max (2*rem + 1) <= uo (since 2*rem <= uo-1). Condition fails.
	// - If rem == uo/2 (exact half): 2*rem == uo.
	//   - Condition becomes: uo + (quo&1) > uo.
	//   - True if quo is odd (bit=1). False if quo is even (bit=0).
	//
	// Overflow Safety:
	// uo <= 2^63 (abs(MinInt64)).
	// rem < uo. Max rem is 2^63 - 1.
	// Max lhs = 2*(2^63-1) + 1 = 2^64 - 1.
	// This fits exactly in uint64.
	if (rem<<1)+(quo&1) > uo {
		quo++
	}

	if quo > math.MaxInt64 {
		if sm == -1 && quo == 1<<63 {
			return Decimal(math.MinInt64)
		}
		panicOverflow()
	}

	return Decimal(int64((quo ^ uint64(sm)) - uint64(sm)))
}
