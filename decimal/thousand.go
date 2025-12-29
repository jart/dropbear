package decimal

// FormatCommas returns the decimal formatted with commas and exactly n decimal places.
func (d Decimal) FormatThousand(n int) string {
	if n < 0 || n > Places {
		panic("illegal places")
	}

	// setup computation
	v := int64(d)
	s := v < 0
	if s {
		if d == Min {
			// handle Min specially: -Min overflows int64
			return formatMinThousand(n)
		}
		v = -v
	}

	// skip the (places - n) least significant fractional digits, with rounding
	skip := max(Places - n, 0)
	if skip > 0 {
		var remainder int64
		for range skip {
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

// formatMinThousand handles FormatThousand() for Min, which can't be negated without overflow.
// Min = -9223372036854775808 = -92233720368.54775808
func formatMinThousand(n int) string {
	const intPart = "-92,233,720,368"
	const full = "-92,233,720,368.54775808"

	if n <= 0 {
		// Truncate to integer, rounding away from zero
		return "-92,233,720,369"
	}
	if n >= Places {
		return full
	}

	// Reuse formatMin logic for rounding, then insert commas
	plain := formatMin(n)
	// plain is like "-9223372036.85"
	// We need "-9,223,372,036.85"

	// Find the decimal point
	dotIdx := -1
	for i, c := range plain {
		if c == '.' {
			dotIdx = i
			break
		}
	}

	var intDigits, fracPart string
	if dotIdx == -1 {
		intDigits = plain[1:] // skip '-'
		fracPart = ""
	} else {
		intDigits = plain[1:dotIdx] // skip '-'
		fracPart = plain[dotIdx:]   // includes '.'
	}

	// Insert commas into integer part
	var result []byte
	result = append(result, '-')
	for i, c := range intDigits {
		if i > 0 && (len(intDigits)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	result = append(result, fracPart...)

	return string(result)
}
