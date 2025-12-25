package decimal

// Precision returns the number of decimal places in this value.
// Useful for determining formatting precision from an increment like 0.01 -> 2.
// For example Decimal.Parse("1.2034").Format(Decimal.Parse("0.01").Precision()) -> "1.20"
// Whereas Parse("1.2034").Quantize(Decimal.Parse("0.01")).String() -> "1.2"
// Whereas Decimal.Parse("1.2034").String() -> "1.2034"
func (d Decimal) Precision() int {
	v := int64(d)
	if v < 0 {
		v = -v
	}
	n := Places
	for n > 0 && v%10 == 0 {
		v /= 10
		n--
	}
	return n
}

// FormatCommas returns the decimal formatted with commas and exactly n decimal places.
func (d Decimal) FormatCommas(n int) string {
	v := int64(d)
	s := v < 0
	if s {
		v = -v
	}

	// skip the (places - n) least significant fractional digits, with rounding
	skip := Places - n
	if skip < 0 {
		skip = 0
	}
	if skip > 0 {
		var remainder int64
		for j := 0; j < skip; j++ {
			remainder = v % 10
			v /= 10
		}
		if remainder >= 5 {
			v++
		}
	}

	isZero := v == 0

	// extract fractional part
	var frac int64
	for j := 0; j < n && j < Places; j++ {
		frac += (v % 10) * pow10[j]
		v /= 10
	}

	// format integer part with commas
	var b [40]byte
	i := len(b)

	if v == 0 {
		i--
		b[i] = '0'
	} else {
		digits := 0
		for v > 0 {
			if digits > 0 && digits%3 == 0 {
				i--
				b[i] = ','
			}
			i--
			b[i] = byte(v%10) + '0'
			v /= 10
			digits++
		}
	}

	if s && !isZero {
		i--
		b[i] = '-'
	}

	intPart := string(b[i:])

	if n == 0 {
		return intPart
	}

	// format fractional part
	var fb [10]byte
	fi := len(fb)
	for j := 0; j < n; j++ {
		fi--
		fb[fi] = byte(frac%10) + '0'
		frac /= 10
	}

	return intPart + "." + string(fb[fi:])
}

// Format returns the decimal formatted with exactly n decimal places, zero-padded.
// For example Decimal.Parse("1.2034").Format(2) -> 13.20"
// Whereas Decimal.Parse("1.2034").String() -> "1.2034"
func (d Decimal) Format(n int) string {
	var b [32]byte
	i := len(b)
	v := int64(d)
	s := v < 0
	if s {
		v = -v
	}

	// skip the (places - n) least significant fractional digits, with rounding
	skip := Places - n
	if skip < 0 {
		skip = 0
	}
	if skip > 0 {
		// check if we need to round up
		var remainder int64
		for j := 0; j < skip; j++ {
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
