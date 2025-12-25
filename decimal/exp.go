package decimal

import "math"

// Exp returns e^d.
func (d Decimal) Exp() Decimal {
	// there's no point in doing fixed point manually
	return Decimal(int64(math.Exp(d.Float64())*Scale + .5))
}
