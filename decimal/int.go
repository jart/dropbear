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

// Int64 returns the integer value of d.
// This method panics if d has a fractional part.
func (d Decimal) Int64() int64 {
	if int64(d)%Scale != 0 {
		panic("decimal: Int64 called on non-integer value")
	}
	return int64(d) / Scale
}

// Int returns the integer value of d.
// This method panics if d has a fractional part or if the result overflows int.
func (d Decimal) Int() int {
	if int64(d)%Scale != 0 {
		panic("decimal: Int called on non-integer value")
	}
	r := int64(d) / Scale
	if r > math.MaxInt || r < math.MinInt {
		panic("decimal overflow")
	}
	return int(r)
}
