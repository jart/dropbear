package decimal

import "math"

// Exp returns e^d, panicking on overflow.
// Values less than -21.416413017 will silently underflow to zero.
func (d Decimal) Exp() Decimal {
	if d > 22945006538 {
		panicOverflow()
	}
	result := math.Exp(d.Float64()) * Scale
	return Decimal(int64(result + .5))
}
