package decimal

import "math"

// Log returns ln(d).
func (d Decimal) Log() Decimal {
	return Decimal(int64(math.Log(d.Float64())*Scale + .5))
}
