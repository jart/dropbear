package black76

import "math"

// Call returns the Black-76 price of a European call on a future.
// - F is the forward price of the underlying future
// - K is the option strike
// - r is the risk free rate
// - T is the time to expiration in years
// - sigma is the volatility
func Call(F, K, r, T, sigma float64) float64 {
	if T <= 0 || sigma <= 0 {
		return math.Max(F-K, 0) * math.Exp(-r*T)
	}
	sqrtT := math.Sqrt(T)
	d1 := (math.Log(F/K) + 0.5*sigma*sigma*T) / (sigma * sqrtT)
	d2 := d1 - sigma*sqrtT
	return math.Exp(-r*T) * (F*normalCDF(d1) - K*normalCDF(d2))
}

// Put returns the Black-76 price of a European put on a future.
// - F is the forward price of the underlying future
// - K is the option strike
// - r is the risk free rate
// - T is the time to expiration in years
// - sigma is the volatility
func Put(F, K, r, T, sigma float64) float64 {
	if T <= 0 || sigma <= 0 {
		return math.Max(K-F, 0) * math.Exp(-r*T)
	}
	sqrtT := math.Sqrt(T)
	d1 := (math.Log(F/K) + 0.5*sigma*sigma*T) / (sigma * sqrtT)
	d2 := d1 - sigma*sqrtT
	return math.Exp(-r*T) * (K*normalCDF(-d2) - F*normalCDF(-d1))
}

// Vega returns the vega of a Black-76 option (dPrice/dSigma).
// - F is the forward price of the underlying future
// - K is the option strike
// - r is the risk free rate
// - T is the time to expiration in years
// - sigma is the volatility
func Vega(F, K, r, T, sigma float64) float64 {
	if T <= 0 || sigma <= 0 {
		return 0
	}
	sqrtT := math.Sqrt(T)
	d1 := (math.Log(F/K) + 0.5*sigma*sigma*T) / (sigma * sqrtT)
	return F * math.Exp(-r*T) * normalPDF(d1) * sqrtT
}

// IV solves for implied volatility using Newton-Raphson with bisection bounds.
// - F is the forward price of the underlying future
// - K is the option strike
// - r is the risk free rate
// - T is the time to expiration in years
// - marketPrice is the observed option price
// - isCall is true for calls, false for puts
func IV(F, K, r, T, marketPrice float64, isCall bool) float64 {
	if T <= 0 || marketPrice <= 0 {
		return 0
	}
	price := func(sigma float64) float64 {
		if isCall {
			return Call(F, K, r, T, sigma)
		}
		return Put(F, K, r, T, sigma)
	}
	// Bisection to find initial bounds [lo, hi] bracketing the solution.
	lo, hi := 0.001, 5.0
	for price(hi) < marketPrice && hi < 100 {
		hi *= 2
	}
	// Newton-Raphson with bisection fallback.
	sigma := (lo + hi) / 2
	for i := 0; i < 100; i++ {
		p := price(sigma)
		diff := p - marketPrice
		if math.Abs(diff) < 1e-10 {
			break
		}
		// Tighten bisection bounds.
		if diff > 0 {
			hi = sigma
		} else {
			lo = sigma
		}
		// Try Newton step.
		vega := Vega(F, K, r, T, sigma)
		if vega > 1e-12 {
			next := sigma - diff/vega
			if next > lo && next < hi {
				sigma = next
				continue
			}
		}
		// Fall back to bisection.
		sigma = (lo + hi) / 2
	}
	return sigma
}

// normalCDF computes the standard normal cumulative distribution function.
func normalCDF(x float64) float64 {
	return 0.5 * math.Erfc(-x/math.Sqrt2)
}

// normalPDF computes the standard normal probability density function.
func normalPDF(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2*math.Pi)
}
