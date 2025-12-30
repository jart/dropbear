package decimal

import "math"

// DivIntEven divides d by n, rounding to the nearest integer.
// Ties (exactly halfway) are rounded to the nearest even integer (Banker's Rounding).
// Panics on overflow (MinInt64 / -1) or division by zero.
func (d Decimal) DivIntEven(n int) Decimal {
	x, y := int64(d), int64(n)

	if x == math.MinInt64 && y == -1 {
		panicOverflow()
	}

	quo := x / y
	rem := x % y

	if rem != 0 {
		// use uint64 for safe magnitude comparison, to handle MinInt64
		// correctly math.MinInt64 has no positive counterpart in int64
		absRem := uint64(rem)
		if rem < 0 {
			absRem = uint64(-rem)
		}
		absY := uint64(y)
		if y < 0 {
			absY = uint64(-y)
		}

		// determine halfway point
		half := absY / 2
		roundAway := false

		if absRem > half {
			// remainder is strictly greater than half: always round away
			roundAway = true
		} else if absRem == half {
			// exact tie (remainder is exactly half of divisor)
			// this can only happen if absY is even
			// if absY is odd, half = floor(y/2), so absRem == half implies absRem < y/2
			if absY&1 == 0 {
				// banksters' rounding: rounds to the nearest *even* integer
				// if current quo is odd, adding 1 (magnitude) makes it even
				// if current quo is even, we leave it alone
				if quo&1 != 0 {
					roundAway = true
				}
			}
		}

		if roundAway {
			if (x < 0) == (y < 0) {
				quo++
			} else {
				quo--
			}
		}
	}

	return Decimal(quo)
}
