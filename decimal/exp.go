package decimal

import "math"

// Exp returns e^d.
// Values greater than 2524762513 will panic due to overflow.
// Values less than -21.416413017 will silently underflow to zero.
func (d Decimal) Exp() Decimal {
	if d > 2524762513 {
		panicOverflow()
	}
	result := math.Exp(d.Float64()) * Scale
	if result >= 9223372036854775807.5 { // MaxInt64 + 0.5
		return Max
	}
	return Decimal(int64(result + .5))
}
