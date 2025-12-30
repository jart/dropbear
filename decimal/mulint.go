package decimal

import (
	"math"
	"math/bits"
)

// MulInt multiplies d by n, panicking on overflow.
func (d Decimal) MulInt(n int) Decimal {
	x, y := int64(d), int64(n)

	// 1. Extract sign and compute absolute values.
	//    This handles the asymmetry of MinInt64 correctly.
	sign := int64(1)
	if (x < 0) != (y < 0) {
		sign = -1
	}

	uX, uY := uint64(x), uint64(y)
	if x < 0 {
		uX = -uX
	}
	if y < 0 {
		uY = -uY
	}

	// 2. Full 128-bit multiply.
	hi, lo := bits.Mul64(uX, uY)

	// 3. Overflow Check
	//    The result must fit in 64 bits (hi == 0) and fit in positive int64 range.
	if hi != 0 || lo > math.MaxInt64 {
		// Allow MinInt64 (1<<63) only if the sign is negative.
		if sign == -1 && hi == 0 && lo == 1<<63 {
			return Decimal(math.MinInt64)
		}
		panicOverflow()
	}

	return Decimal(sign * int64(lo))
}
