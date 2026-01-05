package decimal

import (
	"math"
	"math/bits"
)

// Mul multiplies two decimals.
func (d Decimal) Mul(o Decimal) Decimal {

	// 1. Absolute values and sign
	di := int64(d)
	oi := int64(o)
	dm := di >> 63
	ud := uint64((di ^ dm) - dm)
	om := oi >> 63
	uo := uint64((oi ^ om) - om)
	sm := (di ^ oi) >> 63

	// 2. Perform full 128-bit multiplication: (hi, lo) = ud * uo
	hi, lo := bits.Mul64(ud, uo)

	// 3. Divide the 128-bit product by Scale.
	//    bits.Div64 returns (quo, rem) and panics if quo overflows uint64.
	quo, rem := bits.Div64(hi, lo, uint64(Scale))

	// 4. Bankers' Rounding (Round Half To Even)
	quo -= uint64(int64(Scale/2-(rem+(quo&1))) >> 63)

	// 5. Check for overflow of the signed 64-bit result range.
	//    The quotient must fit in the positive part of int64.
	if quo > math.MaxInt64 {
		if quo == 1<<63 && sm == -1 {
			return Min
		}
		panic("decimal overflow")
	}

	// 6. Apply sign
	return Decimal(int64((quo ^ uint64(sm)) - uint64(sm)))
}
