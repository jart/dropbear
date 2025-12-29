package decimal

import (
	"math"
	"math/bits"
)

// Div divides d by o, rounding half away from zero (Commercial Rounding).
// Use this for pricing, display, and final settlement, e.g. 2.5->3, 3.5->4.
// This is the standard for commerce (invoices, tax calculation, pricing).
// It matches the round() function in Excel and most human expectations.
func (d Decimal) Div(o Decimal) Decimal {
	if o == 0 {
		panic("decimal division by zero")
	}

	// 1. Determine sign.
	sign := int64(1)
	if (d < 0) != (o < 0) {
		sign = -1
	}

	// 2. Absolute values for uint64 arithmetic.
	uD, uO := uint64(d), uint64(o)
	if d < 0 {
		uD = -uD
	}
	if o < 0 {
		uO = -uO
	}

	// 3. Compute 128-bit numerator: d * Scale
	//    bits.Mul64 results in (hi, lo)
	hi, lo := bits.Mul64(uD, uint64(Scale))

	// 4. Divide 128-bit numerator by 64-bit denominator.
	//    bits.Div64(hi, lo, y) returns (quo, rem) = (hi, lo) / y
	//    It panics automatically if the quotient overflows uint64 (which implies
	//    hi >= uO, meaning the result is too big for 64 bits anyway).
	quo, rem := bits.Div64(hi, lo, uO)

	// 5. Rounding to nearest (Half Away From Zero).
	//    We round up if rem >= o/2.
	//    To avoid truncation issues with odd denominators, we use:
	//    rem >= (o + 1) / 2
	if rem >= (uO+1)/2 {
		quo++
	}

	// 6. Check for overflow of the signed 64-bit result.
	if quo > math.MaxInt64 {
		// EDGE CASE: Allow MinInt64 (magnitude 2^63) ONLY if sign is negative.
		// 1<<63 is the uint64 representation of absolute MinInt64.
		if sign == -1 && quo == 1<<63 {
			return Decimal(math.MinInt64)
		}
		panicOverflow()
	}

	return Decimal(sign * int64(quo))
}
