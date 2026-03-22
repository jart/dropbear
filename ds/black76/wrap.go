package black76

import "dropbear/clocky"

// CallDelta computes the delta of a call option.
// This is a convenience wrapper around CallGreeks.
func CallDelta(F, K, r float64, E clocky.Duration, sigma float64) float64 {
	delta, _, _, _ := CallGreeks(F, K, r, E, sigma)
	return delta
}

// PutDelta computes the delta of a put option.
// This is a convenience wrapper around PutGreeks.
func PutDelta(F, K, r float64, E clocky.Duration, sigma float64) float64 {
	delta, _, _, _ := PutGreeks(F, K, r, E, sigma)
	return delta
}

// CallTheta computes the theta of a call option.
// This is a convenience wrapper around CallGreeks.
func CallTheta(F, K, r float64, E clocky.Duration, sigma float64) float64 {
	_, theta, _, _ := CallGreeks(F, K, r, E, sigma)
	return theta
}

// PutTheta computes the theta of a put option.
// This is a convenience wrapper around PutGreeks.
func PutTheta(F, K, r float64, E clocky.Duration, sigma float64) float64 {
	_, theta, _, _ := PutGreeks(F, K, r, E, sigma)
	return theta
}

// Gamma computes the gamma of a call or put option.
// This is a convenience wrapper around CallGreeks.
func Gamma(F, K, r float64, E clocky.Duration, sigma float64) float64 {
	_, _, gamma, _ := CallGreeks(F, K, r, E, sigma)
	return gamma
}

// Vega computes the vega of a call or put option.
// This is a convenience wrapper around CallGreeks.
func Vega(F, K, r float64, E clocky.Duration, sigma float64) float64 {
	_, _, _, vega := CallGreeks(F, K, r, E, sigma)
	return vega
}
