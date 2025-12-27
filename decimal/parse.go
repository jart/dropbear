package decimal

import "math"

const (
	maxInt      = math.MaxInt64 / Scale
	minInt      = math.MinInt64 / Scale
	maxIntDiv10 = maxInt / 10
	minIntDiv10 = minInt / 10
)

// Parse parses a decimal string like "123.45" or "3.6372083e-07".
// More than 9 decimal places is truncated (not rounded).
// Panics on invalid input or integer overflow.
func Parse(str string) Decimal {
	if len(str) == 0 {
		panic("decimal.Parse: empty string")
	}

	// parse sign
	i := 0
	s := int64(1)
	switch str[0] {
	case '-':
		s = -1
		i++
	case '+':
		i++
	}

	// parse integer part
	digits := 0
	x := int64(0)
	for i < len(str) {
		if str[i] >= '0' && str[i] <= '9' {
			// sorry you can't do stuff like 9'223'372'036.854'775'807e-9
			if x > maxIntDiv10 || x < minIntDiv10 {
				panic("decimal.Parse overflow in " + str)
			}
			x *= 10
			y := int64(str[i]-'0') * s
			x += y
			digits++
			i++
		} else if str[i] == '_' || str[i] == ',' || str[i] == '\'' {
			i++
		} else {
			break
		}
	}
	if x > maxInt || x < minInt {
		panic("decimal.Parse overflow in " + str)
	}

	// parse fractional part (only read up to Places digits to avoid overflow)
	if i < len(str) && str[i] == '.' {
		i++
		f := int64(0)
		k := 0
		for i < len(str) {
			if str[i] >= '0' && str[i] <= '9' {
				if k < Places {
					f *= 10
					f += int64(str[i] - '0')
					k++
				}
				digits++
				i++
			} else if str[i] == '_' || str[i] == ',' || str[i] == '\'' {
				i++
			} else {
				break
			}
		}
		if k < Places {
			f *= pow10[Places-k]
		}
		x = x*Scale + f*s
	} else {
		x *= Scale
	}

	if digits == 0 {
		panic("decimal.Parse: no digits in " + str)
	}

	// parse exponent
	if i < len(str) && (str[i] == 'e' || str[i] == 'E') {
		i++
		expSign := int64(1)
		if i < len(str) && str[i] == '-' {
			expSign = -1
			i++
		} else if i < len(str) && str[i] == '+' {
			i++
		}
		exp := int64(0)
		for i < len(str) && str[i] >= '0' && str[i] <= '9' {
			exp = exp*10 + int64(str[i]-'0')
			i++
		}
		exp *= expSign
		if exp > 0 {
			for exp > 0 {
				shift := exp
				if shift >= int64(len(pow10)) {
					shift = int64(len(pow10) - 1)
				}
				x *= pow10[shift]
				exp -= shift
			}
		} else if exp < 0 {
			exp = -exp
			for exp > 0 {
				shift := exp
				if shift >= int64(len(pow10)) {
					shift = int64(len(pow10) - 1)
				}
				x /= pow10[shift]
				exp -= shift
			}
		}
	}

	if i != len(str) {
		panic("decimal.Parse: trailing garbage in " + str)
	}

	return Decimal(x)
}
