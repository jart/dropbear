package decimal

import "math"

// DivInt divides d by n, rounding to the nearest integer.
// Panics on overflow (MinInt64 / -1) or division by zero.
func (d Decimal) DivInt(n int) Decimal {
	x, y := int64(d), int64(n)

	if x == math.MinInt64 && y == -1 {
		panicOverflow()
	}

	quo := x / y
	rem := x % y

	if rem != 0 {
		absRem := rem
		if absRem < 0 {
			absRem = -absRem
		}
		absY := y
		if absY < 0 {
			absY = -absY
		}

		// calculate threshold safely to avoid overflow when absY == MaxInt64.
		// we want ceil(absY / 2). This gives us that without adding 1 to MaxInt64.
		threshold := (absY >> 1) + (absY & 1)

		if absRem >= threshold {
			if (x < 0) == (y < 0) {
				quo++
			} else {
				quo--
			}
		}
	}

	return Decimal(quo)
}
