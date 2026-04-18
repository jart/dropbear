package norm

import "math"

const sqrt2Pi = math.Sqrt2 * math.SqrtPi // sqrt(2π)

// PDF computes the standard normal probability density function.
func PDF(x float64) float64 {
	return math.Exp(-.5*x*x) / sqrt2Pi
}
