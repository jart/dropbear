package decimal

import (
	"math"
	"math/bits"
)

// Mul multiplies two decimals, panicking on overflow.
func (d Decimal) Mul(o Decimal) Decimal {

	// 1. Determine the sign of the result.
	sign := int64(1)
	if (d < 0) != (o < 0) {
		sign = -1
	}

	// 2. Convert to absolute values as uint64 to safely handle MinInt64.
	//    Note: -uint64(MinInt64) is 2^63, which fits in uint64.
	uD, uO := uint64(d), uint64(o)
	if d < 0 {
		uD = -uD
	}
	if o < 0 {
		uO = -uO
	}

	// 3. Perform full 128-bit multiplication: (hi, lo) = uD * uO
	hi, lo := bits.Mul64(uD, uO)

	// 4. Divide the 128-bit product by Scale.
	//    bits.Div64 returns (quo, rem) and panics if quo overflows uint64.
	//    We discard 'rem' (truncation), or use it for rounding if desired.
	quo, _ := bits.Div64(hi, lo, uint64(Scale))

	// 5. Check for overflow of the signed 64-bit result range.
	//    The quotient must fit in the positive part of int64.
	if quo > math.MaxInt64 {
		if quo == 9223372036854775808 && sign == -1 {
			return Min
		}
		panicOverflow()
	}

	return Decimal(sign * int64(quo))
}
