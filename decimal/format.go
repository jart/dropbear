package decimal

import (
	"math"
	"math/big"
)

// Format returns the decimal formatted with exactly n decimal places, zero-padded.
// For example Decimal.Parse("1.2034").Format(2) -> 13.20"
// Whereas Decimal.Parse("1.2034").String() -> "1.2034"
func (d Decimal) Format(n int) string {
	if n < 0 || n > Places {
		panic("illegal places")
	}

	// setup computation
	var b [32]byte
	i := len(b)
	v := int64(d)
	s := v < 0
	if s {
		if d == Min {
			// handle Min specially: -Min overflows int64
			// Min = -9223372036854775808 = -9223372036.854775808
			// we process the digits directly without negation
			return formatMin(n)
		}
		v = -v
	}

	// skip the (places - n) least significant fractional digits, with rounding
	skip := max(Places-n, 0)
	if skip > 0 {
		// check if we need to round up
		var remainder int64
		for range skip {
			remainder = v % 10
			v /= 10
		}
		if remainder >= 5 {
			v++
		}
	}

	// after rounding, check if value is zero (don't output negative zero)
	isZero := v == 0

	// write n fractional digits right-to-left
	for j := 0; j < n && j < Places; j++ {
		i--
		b[i] = byte(v%10) + '0'
		v /= 10
	}

	// write decimal point if we have fractional digits
	if n > 0 {
		i--
		b[i] = '.'
	}

	// write integer digits right-to-left
	if v == 0 {
		i--
		b[i] = '0'
	} else {
		for v > 0 {
			i--
			b[i] = byte(v%10) + '0'
			v /= 10
		}
	}

	// write sign (but not for zero)
	if s && !isZero {
		i--
		b[i] = '-'
	}

	return string(b[i:])
}

//go:noinline
func formatMin(n int) string {
	r := big.NewRat(math.MinInt64, Scale)
	return r.FloatString(n)
}
