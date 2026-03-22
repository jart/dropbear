package black76

import "math"

const sqrt2Pi = math.Sqrt2 * math.SqrtPi // sqrt(2π)

// NormCDF computes the standard normal cumulative distribution function.
func NormCDF(x float64) float64 {
	return .5 * math.Erfc(-x/math.Sqrt2)
}

// NormPDF computes the standard normal probability density function.
func NormPDF(x float64) float64 {
	return math.Exp(-.5*x*x) / sqrt2Pi
}
