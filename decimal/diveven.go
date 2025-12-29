package decimal

import (
	"math"
	"math/bits"
)

// DivEven divides d by o, rounding half to even (Banker's Rounding).
// Use this for statistical aggregation, averages, and algorithmic strategies
// to minimize cumulative rounding bias. For example: 2.5 -> 2, 3.5 -> 4.
func (d Decimal) DivEven(o Decimal) Decimal {

	sign := int64(1)
	if (d < 0) != (o < 0) {
		sign = -1
	}

	uD, uO := uint64(d), uint64(o)
	if d < 0 {
		uD = -uD
	}
	if o < 0 {
		uO = -uO
	}

	// 128-bit Mul
	hi, lo := bits.Mul64(uD, uint64(Scale))

	// Check for multiplication overflow (result > 2^64)
	if hi >= uint64(Scale) {
		panicOverflow()
	}

	// Division: quo = result, rem = remainder
	quo, rem := bits.Div64(hi, lo, uO)

	// Efficient half comparison using threshold similar to DivInt fix
	// threshold = ceil(uO / 2)
	threshold := (uO >> 1) + (uO & 1)

	var comp int8
	if rem > threshold {
		comp = 1
	} else if rem == threshold {
		// If uO is odd, threshold is slightly > uO/2.
		// e.g. uO=5, threshold=3. rem=3 -> 6 > 5. strict greater.
		// If uO is even, threshold is exactly uO/2.
		if uO&1 == 1 {
			comp = 1 // Odd divisor means rem can't be exactly half
		} else {
			comp = 0 // Exact half
		}
	} else {
		// rem < threshold.
		// If uO is odd (5), threshold is 3. rem=2. 2 < 2.5. Less.
		// If uO is even (4), threshold is 2. rem=1. 1 < 2. Less.
		comp = -1
	}

	if comp > 0 || (comp == 0 && (quo&1) == 1) {
		quo++
	}

	if quo > math.MaxInt64 {
		if sign == -1 && quo == 9223372036854775808 {
			return Decimal(math.MinInt64)
		}
		panicOverflow()
	}

	return Decimal(sign * int64(quo))
}
