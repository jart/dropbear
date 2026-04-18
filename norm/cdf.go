package norm

import "math"

// CDF computes the standard normal cumulative distribution function.
func CDF(x float64) float64 {
	return .5 * math.Erfc(-x/math.Sqrt2)
}
