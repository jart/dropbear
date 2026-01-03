package decimal

import "strings"

// FormatThousand returns the decimal formatted with commas and exactly n decimal places.
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
	skip := max(Places-n, 0)
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
// Uses formatMin for the value, then inserts commas.
func formatMinThousand(n int) string {
	s := formatMin(n)

	// Find decimal point position
	dotIdx := strings.Index(s, ".")
	var intPart, fracPart string
	if dotIdx == -1 {
		intPart = s
		fracPart = ""
	} else {
		intPart = s[:dotIdx]
		fracPart = s[dotIdx:]
	}

	// Extract sign and digits
	sign := ""
	if intPart[0] == '-' {
		sign = "-"
		intPart = intPart[1:]
	}

	// Insert commas into integer part
	var result []byte
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}

	return sign + string(result) + fracPart
}
