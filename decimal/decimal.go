package decimal

import (
	"math"
)

// Decimal represents a fixed-point decimal number.
type Decimal int64

const (
	Zero    = Decimal(0)
	One     = Decimal(Scale)
	Max     = Decimal(math.MaxInt64) // +9'223'372'036'854.775807
	Min     = Decimal(math.MinInt64) // -9'223'372'036'854.775808
	Epsilon = Decimal(1)             // 0.000001
	Scale   = 1_000_000
	Places  = 6
)

// Add returns d + o.
// This method panics if the result overflows.
func (x Decimal) Add(y Decimal) Decimal {
	z := x + y
	if ((z ^ x) & (z ^ y)) < 0 {
		panic("decimal overflow")
	}
	return z
}

// Sub returns d - o.
// This method panics if the result overflows.
func (x Decimal) Sub(y Decimal) Decimal {
	z := x - y
	if ((x ^ y) & (z ^ x)) < 0 {
		panic("decimal overflow")
	}
	return z
}

// Neg returns -d.
// This method panics if the result overflows.
func (d Decimal) Neg() Decimal {
	if d == Min {
		panic("decimal overflow")
	}
	return -d
}

// Sign returns -1 for negative numbers, 0 for zero, and +1 for positive numbers.
func (d Decimal) Sign() Decimal {
	if d > 0 {
		return One
	}
	if d < 0 {
		return NegOne
	}
	return Zero
}

// IsPositive returns true for positive non-zero numbers.
func (d Decimal) IsPositive() bool {
	return d > 0
}

// IsNegative returns true for negative non-zero numbers.
func (d Decimal) IsNegative() bool {
	return d < 0
}

// IsZero returns true only for zero.
func (d Decimal) IsZero() bool {
	return d == 0
}

// Sqr returns d*d.
func (d Decimal) Sqr() Decimal {
	return d.Mul(d)
}

// Cmp compares d and o and returns -1 if d < o, 0 if d == o, 1 if d > o.
func (d Decimal) Cmp(o Decimal) int {
	if d < o {
		return -1
	}
	if d > o {
		return 1
	}
	return 0
}

// Abs returns the absolute value of d.
// This method panics if the result overflows.
func (d Decimal) Abs() Decimal {
	if d < 0 {
		if d == Min {
			panic("decimal overflow")
		}
		return -d
	}
	return d
}

// Min returns the smaller of d and o.
func (d Decimal) Min(o Decimal) Decimal {
	if d < o {
		return d
	}
	return o
}

// Max returns the larger of d and o.
func (d Decimal) Max(o Decimal) Decimal {
	if d > o {
		return d
	}
	return o
}

// Compare compares a to b.
func Compare(a, b Decimal) int {
	return a.Cmp(b)
}

// CompareReverse compares b to a.
func CompareReverse(a, b Decimal) int {
	return b.Cmp(a)
}
