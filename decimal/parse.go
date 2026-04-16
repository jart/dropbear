package decimal

import (
	"errors"
	"math"
	"unsafe"
)

var (
	ErrEmptyNumber   = errors.New("empty number")
	ErrIllegalNumber = errors.New("illegal number")
	ErrMissingNumber = errors.New("missing number")
	ErrToastedNumber = errors.New("toasted number")
	ErrBrokenNumber  = errors.New("broken number")
)

// Parse parses a decimal string.
func Parse(str string) Decimal {
	res, err := ParseBytes(unsafe.Slice(unsafe.StringData(str), len(str)))
	if err != nil {
		panic(err)
	}
	return res
}

// ParseString parses a decimal string.
func ParseString(str string) (Decimal, error) {
	return ParseBytes(unsafe.Slice(unsafe.StringData(str), len(str)))
}

// ParseBytes parses a decimal from a byte slice.
func ParseBytes(str []byte) (Decimal, error) {
	if len(str) == 0 {
		return 0, ErrEmptyNumber
	}

	i := 0
	neg := false
	switch str[0] {
	case '-':
		neg = true
		i++
	case '+':
		i++
	}

	// Accumulate coef as the unsigned absolute value. The ceiling depends
	// on sign: a positive Decimal can hold up to MaxInt64, but a negative
	// one can hold |MinInt64| = MaxInt64 + 1 = 2^63.
	maxAbs := uint64(math.MaxInt64)
	if neg {
		maxAbs++
	}

	coef := uint64(0)
	exp := int64(0)
	digits := 0
	dot := -1

	// For banker's rounding of digits dropped past coef's capacity:
	// extraDigit is the first such dropped digit (-1 if none),
	// hasMoreDigits is true if any non-zero digit follows it.
	extraDigit := int64(-1)
	hasMoreDigits := false

	// 1. Parse main number part
	for i < len(str) {
		c := str[i]
		if c >= '0' && c <= '9' {
			digit := uint64(c - '0')
			if extraDigit >= 0 {
				// Already overflowed: subsequent digits would corrupt
				// coef's positional value, so just remember the tail
				// for rounding.
				if digit > 0 {
					hasMoreDigits = true
				}
			} else if coef > (maxAbs-digit)/10 {
				if dot < 0 {
					return 0, ErrIllegalNumber
				}
				// Fractional overflow: drop the digit but remember it.
				// Do NOT change exp -- coef * 10^exp still represents
				// the same value (we just lost the tail).
				extraDigit = int64(digit)
			} else {
				coef = coef*10 + digit
				if dot >= 0 {
					exp--
				}
			}
			digits++
			i++
		} else if c == '.' {
			if dot >= 0 {
				return 0, ErrBrokenNumber
			}
			dot = i
			i++
		} else if c == '_' || c == ',' || c == '\'' {
			i++
		} else {
			break
		}
	}

	if digits == 0 {
		return 0, ErrMissingNumber
	}

	// 2. Parse exponent part
	if i < len(str) && (str[i] == 'e' || str[i] == 'E') {
		i++
		es := int64(1)
		if i < len(str) {
			switch str[i] {
			case '-':
				es = -1
				i++
			case '+':
				i++
			}
		}
		e := int64(0)
		hasExpDigits := false
		for i < len(str) && str[i] >= '0' && str[i] <= '9' {
			if e > 1000 {
				e = 1001
			} else {
				e = e*10 + int64(str[i]-'0')
			}
			hasExpDigits = true
			i++
		}
		if hasExpDigits {
			exp += e * es
		}
	}

	if i != len(str) {
		return 0, ErrBrokenNumber
	}

	// 3. Final Alignment and Rounding
	shift := exp + Places

	if exp > 18 {
		if coef == 0 {
			return 0, nil
		}
		return 0, ErrIllegalNumber
	}
	if exp < -30 {
		return 0, nil
	}

	finalize := func(c uint64) (Decimal, error) {
		if c > maxAbs {
			return 0, ErrIllegalNumber
		}
		if neg {
			// -int64(c) yields the right value even when c == 1<<63
			// (unsigned negation wraps to MinInt64 as int64).
			return Decimal(-int64(c)), nil
		}
		return Decimal(int64(c)), nil
	}

	if shift == 0 {
		// extraDigit (if set) is the first digit dropped past coef.
		// It sits at position 10^(exp-1) -- one decimal place past the
		// least significant digit of coef -- so it represents the
		// fractional remainder when rounding to a Decimal.
		if extraDigit > 5 || (extraDigit == 5 && (hasMoreDigits || coef%2 != 0)) {
			coef++
		}
		return finalize(coef)
	}

	if shift > 0 {
		// If a digit was dropped during fractional accumulation, those
		// digits sit at integer positions of the result and cannot be
		// recovered, so the value is unrepresentable.
		if extraDigit >= 0 {
			if exp >= 0 {
				return 0, ErrIllegalNumber
			}
			return 0, ErrToastedNumber
		}
		for shift > 0 {
			p := shift
			if p >= int64(len(pow10)) {
				p = int64(len(pow10) - 1)
			}
			mul := uint64(pow10[p])
			if coef > maxAbs/mul {
				if exp >= 0 {
					return 0, ErrIllegalNumber
				}
				return 0, ErrToastedNumber
			}
			coef *= mul
			shift -= p
		}
		return finalize(coef)
	}

	// Negative shift: divide and round
	divisorExp := -shift
	if divisorExp < int64(len(pow10)) {
		divisor := uint64(pow10[divisorExp])
		rem := coef % divisor
		coef /= divisor

		// Banker's Rounding
		half := divisor / 2
		isMid := rem == half

		// Digits dropped past coef's capacity push us strictly above
		// the midpoint when rem == half.
		if isMid && (extraDigit > 0 || hasMoreDigits) {
			rem++
			isMid = false
		}

		if rem > half || (isMid && coef%2 != 0) {
			coef++
		}
		return finalize(coef)
	}

	// Divisor 10^divisorExp doesn't fit in int64 (divisorExp >= 19).
	// For divisorExp == 19: half = 5e18 still fits, and coef < 10^19,
	// so coef/divisor = 0 and we just compare coef against half.
	// For divisorExp > 19: half > MaxInt64 >= coef, so result is 0.
	if divisorExp == int64(len(pow10)) {
		half := 5 * uint64(pow10[len(pow10)-1])
		if coef > half || (coef == half && (extraDigit > 0 || hasMoreDigits)) {
			return finalize(1)
		}
	}
	return 0, nil
}
