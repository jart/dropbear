package decimal

import "math"

// maxSafeFloat is the largest float64 value that can be converted to Decimal
// without precision loss. Beyond this, float64 can't represent values with
// nanosecond precision (our smallest unit). This is 2^53 / Scale.
const maxSafeFloat = float64(1<<53) / Scale // ~9,007,199

// FromFloat64 converts float64 to Decimal.
// Panics on NaN, infinity, or values where float64 lacks sufficient precision.
// For large values (above ~9 million), use Parse() with a string instead.
func FromFloat64(n float64) Decimal {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		panic("decimal: NaN or infinity")
	}
	if n > maxSafeFloat || n < -maxSafeFloat {
		panic("decimal: float64 lacks precision at this scale")
	}
	return Decimal(math.Round(n * Scale))
}

// Float64 converts Decimal to float64.
// This may lose precision for very large values.
func (d Decimal) Float64() float64 {
	return float64(d) / Scale
}
