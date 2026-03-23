package prob

import "math"

// NormCDF computes the standard normal cumulative distribution function.
func NormCDF(x float64) float64 {
	return .5 * math.Erfc(-x/math.Sqrt2)
}
