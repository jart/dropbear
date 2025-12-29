package decimal

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
	skip := max(Places - n, 0)
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

// formatMin handles Format() for Min, which can't be negated without overflow.
// Min = -9223372036854775808 = -92233720368.54775808
func formatMin(n int) string {
	// Full representation with 8 decimal places
	const full = "-92233720368.54775808"
	const intPart = "-92233720368"

	if n <= 0 {
		// Truncate to integer, rounding away from zero
		// .54775808 >= .5, so round to -92233720369
		return "-92233720369"
	}
	if n >= Places {
		return full
	}

	// Build result with n decimal places and rounding
	// Digits after decimal: 54775808
	diglet := []byte{'5', '4', '7', '7', '5', '8', '0', '8'}
	result := intPart + "."

	// Check if we need to round up (away from zero = more negative = add 1 to magnitude)
	roundUp := n < len(diglet) && diglet[n] >= '5'

	// Take first n digits
	frac := string(diglet[:n])

	if roundUp {
		// Add 1 to the fractional part, propagating carry if needed
		fracBytes := []byte(frac)
		carry := true
		for i := len(fracBytes) - 1; i >= 0 && carry; i-- {
			if fracBytes[i] < '9' {
				fracBytes[i]++
				carry = false
			} else {
				fracBytes[i] = '0'
			}
		}
		if carry {
			// Carry propagated into integer part
			// -92233720368.5... rounds to -92233720369.0...
			return "-92233720369." + string(make([]byte, n)) // n zeros
		}
		frac = string(fracBytes)
	}

	return result + frac
}
