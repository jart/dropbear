package decimal

import "math"

const (
	maxInt      = math.MaxInt64 / Scale
	minInt      = math.MinInt64 / Scale
	maxIntDiv10 = maxInt / 10
	minIntDiv10 = minInt / 10
)

// Parse parses a decimal string like "123.45" or "3.6372083e-07".
// Trimmed to 8 decimal places using Bankers' Rounding.
// Panics on invalid input or integer overflow.
func Parse(str string) Decimal {
	if len(str) == 0 {
		panic("empty number")
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
				panic("illegal number")
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
		panic("forbidden number")
	}

	// parse fractional part (read Places+1 digits for rounding)
	if i < len(str) && str[i] == '.' {
		i++
		f := int64(0)
		k := 0
		extraDigit := int64(-1) // -1 means no extra digit
		hasMoreDigits := false  // non-zero digits after extraDigit?
		for i < len(str) {
			if str[i] >= '0' && str[i] <= '9' {
				if k < Places {
					f *= 10
					f += int64(str[i] - '0')
					k++
				} else if k == Places {
					extraDigit = int64(str[i] - '0')
					k++
				} else if str[i] != '0' {
					hasMoreDigits = true
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
		// apply banker's rounding based on extra digit
		if extraDigit > 5 {
			f++
		} else if extraDigit == 5 {
			// round to even only if exactly at midpoint
			if hasMoreDigits || f%2 != 0 {
				f++
			}
		}
		x = x*Scale + f*s
	} else {
		x *= Scale
	}
	if digits == 0 {
		panic("missing number")
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
				mul := pow10[shift]
				if x > math.MaxInt64/mul || x < math.MinInt64/mul {
					panic("toasted number")
				}
				x *= mul
				exp -= shift
			}
		} else if exp < 0 {
			exp = -exp
			for exp > 0 {
				shift := exp
				if shift >= int64(len(pow10)) {
					shift = int64(len(pow10) - 1)
				}
				divisor := pow10[shift]
				rem := x % divisor
				x /= divisor
				// banker's rounding on the remainder
				half := divisor / 2
				if rem < 0 {
					rem = -rem
				}
				if rem > half || (rem == half && x%2 != 0) {
					if s > 0 {
						x++
					} else {
						x--
					}
				}
				exp -= shift
			}
		}
	}

	if i != len(str) {
		panic("broken number")
	}

	return Decimal(x)
}
