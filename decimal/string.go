package decimal

import (
	"math"
	"math/big"
	"strings"
)

// String returns the decimal as a string with trailing zeros trimmed.
func (d Decimal) String() string {
	if d == 0 {
		return "0"
	}
	var b [24]byte
	return string(d.Append(b[:0]))
}

// GoString returns Go syntax for the decimal, e.g. decimal.Parse("22.36").
func (d Decimal) GoString() string {
	return "decimal.Parse(\"" + d.String() + "\")"
}

// Append appends the string representation of d to dst and returns the result.
func (d Decimal) Append(dst []byte) []byte {

	// min needs special care
	if d == Min {
		return append(dst, stringifyMin()...)
	}

	// prepare for computation
	var b [32]byte
	i := len(b)
	v := int64(d)
	s := v < 0
	if s {
		v = -v
	}

	// write fractional digits right-to-left, trimming trailing zeros
	t := true
	for range Places {
		c := byte(v%10) + '0'
		v /= 10
		if t && c == '0' {
			continue
		}
		t = false
		i--
		b[i] = c
	}

	// write decimal point if we have fractional digits
	if !t {
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

	// write sign
	if s {
		i--
		b[i] = '-'
	}

	return append(dst, b[i:]...)
}

//go:noinline
func stringifyMin() string {
	r := big.NewRat(math.MinInt64, Scale)
	str := r.FloatString(Places)
	str = strings.TrimRight(str, "0")
	if str[len(str)-1] == '.' {
		str = str[:len(str)-1]
	}
	return str
}
