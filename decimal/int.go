package decimal

import "math"

// FromInt converts int to Decimal.
func FromInt(n int) Decimal {
	if n > math.MaxInt64/Scale || n < math.MinInt64/Scale {
		panic("decimal overflow")
	}
	return Decimal(int64(n) * Scale)
}

// FromInt64 converts int64 to Decimal.
func FromInt64(n int64) Decimal {
	if n > math.MaxInt64/Scale || n < math.MinInt64/Scale {
		panic("decimal overflow")
	}
	return Decimal(n * Scale)
}

// Int64 returns the integer part of d, rounding toward zero.
func (d Decimal) Int64() int64 {
	return int64(d) / Scale
}

// Int returns the integer part of d, rounding toward zero.
// This method panics if the result overflows int.
func (d Decimal) Int() int {
	r := int64(d) / Scale
	if r > math.MaxInt || r < math.MinInt {
		panic("decimal overflow")
	}
	return int(r)
}
