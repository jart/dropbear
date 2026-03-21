// black76 lets you compute the price and greeks of European-style
// options contracts using the price of the underlying futures.
//
// It's called Black-76 because Fischer Black published it solo in his
// 1976 paper called "The Pricing of Commodity Contracts." Scholes
// wasn't a co-author on that paper. The original Black-Scholes (1973)
// was Black and Scholes working together (with Merton contributing
// independently) discovering the math for pricing equity options.
//
// It took Black three years to figure out that if the underlying is a
// futures contract, you can simplify the math because the futures price
// already embeds the cost of carry. You just discount the expected
// payoff using the forward price directly.
package black76

import "math"

const sqrt2Pi = math.Sqrt2 * math.SqrtPi // sqrt(2π)

// Call returns the price of a European call on a future.
// - F is the forward price of the underlying future
// - K is the option strike
// - r is the risk free rate (e.g. 0.05 for 5%)
// - T is the time to expiration in years
// - sigma is the volatility
func Call(F, K, r, T, sigma float64) float64 {
	if T <= 0 || sigma <= 0 {
		return math.Max(F-K, 0) * math.Exp(-r*T)
	}
	sqrtT := math.Sqrt(T)
	sigmaT := sigma * sqrtT
	d1 := math.FMA(.5, sigmaT, math.Log(F/K)/sigmaT)
	d2 := d1 - sigmaT
	return math.Exp(-r*T) * (F*normalCDF(d1) - K*normalCDF(d2))
}

// Put returns the price of a European put on a future.
// - F is the forward price of the underlying future
// - K is the option strike
// - r is the risk free rate (e.g. 0.05 for 5%)
// - T is the time to expiration in years
// - sigma is the volatility
func Put(F, K, r, T, sigma float64) float64 {
	if T <= 0 || sigma <= 0 {
		return math.Max(K-F, 0) * math.Exp(-r*T)
	}
	sqrtT := math.Sqrt(T)
	sigmaT := sigma * sqrtT
	d1 := math.FMA(.5, sigmaT, math.Log(F/K)/sigmaT)
	d2 := d1 - sigma*sqrtT
	return math.Exp(-r*T) * (K*normalCDF(-d2) - F*normalCDF(-d1))
}

// Vega returns the vega of an option (dPrice/dSigma).
// - F is the forward price of the underlying future
// - K is the option strike
// - r is the risk free rate (e.g. 0.05 for 5%)
// - T is the time to expiration in years
// - sigma is the volatility
func Vega(F, K, r, T, sigma float64) float64 {
	if T <= 0 || sigma <= 0 {
		return 0
	}
	sqrtT := math.Sqrt(T)
	sigmaT := sigma * sqrtT
	d1 := math.FMA(.5, sigmaT, math.Log(F/K)/sigmaT)
	return F * math.Exp(-r*T) * normalPDF(d1) * sqrtT
}

// IV solves for implied volatility.
// - F is the forward price of the underlying future
// - K is the option strike
// - r is the risk free rate (e.g. 0.05 for 5%)
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
	// bisection to find initial bounds [lo, hi] bracketing the solution
	lo, hi := 0.001, 5.0
	for price(hi) < marketPrice && hi < 100 {
		hi *= 2
	}
	// halley's method with bisection fallback
	// converges cubically so we need far fewer iterations than newton
	// uses vomma (d²price/dσ²) = vega·d1·d2/σ which is free since we
	// already have d1 and d2; the halley step simplifies to:
	//   σ' = σ - 2·f·f' / (2·f'² - f·f'')
	//      = σ - 2·diff / (2·vega - diff·d1·d2/σ)
	disc := math.Exp(-r * T)
	sqrtT := math.Sqrt(T)
	logFK := math.Log(F / K)
	sigma := (lo + hi) / 2
	for range 100 {
		sigmaT := sigma * sqrtT
		d1 := math.FMA(.5, sigmaT, logFK/sigmaT)
		d2 := d1 - sigmaT
		// compute price inline to reuse d1/d2
		var p float64
		if isCall {
			p = disc * (F*normalCDF(d1) - K*normalCDF(d2))
		} else {
			p = disc * (K*normalCDF(-d2) - F*normalCDF(-d1))
		}
		diff := p - marketPrice
		if math.Abs(diff) < 1e-10 {
			break
		}
		// tighten bisection bounds
		if diff > 0 {
			hi = sigma
		} else {
			lo = sigma
		}
		// try halley step
		vega := F * disc * normalPDF(d1) * sqrtT
		if vega > 1e-12 {
			next := sigma - 2*diff/(2*vega-diff*d1*d2/sigma)
			if next > lo && next < hi {
				sigma = next
				continue
			}
		}
		// fall back to bisection
		sigma = (lo + hi) / 2
	}
	return sigma
}

// Gamma returns the gamma of an option (d²Price/dF²).
// Gamma is the same for calls and puts.
func Gamma(F, K, r, T, sigma float64) float64 {
	if T <= 0 || sigma <= 0 || F <= 0 {
		return 0
	}
	sqrtT := math.Sqrt(T)
	sigmaT := sigma * sqrtT
	d1 := math.FMA(.5, sigmaT, math.Log(F/K)/sigmaT)
	return math.Exp(-r*T) * normalPDF(d1) / (F * sigma * sqrtT)
}

// CallTheta returns the daily theta of a call (dPrice/dT per day).
func CallTheta(F, K, r, T, sigma float64) float64 {
	if T <= 0 || sigma <= 0 {
		return 0
	}
	sqrtT := math.Sqrt(T)
	sigmaT := sigma * sqrtT
	d1 := math.FMA(.5, sigmaT, math.Log(F/K)/sigmaT)
	d2 := d1 - sigmaT
	disc := math.Exp(-r * T)
	theta := -F*disc*normalPDF(d1)*sigma/(2*sqrtT) +
		r*disc*(F*normalCDF(d1)-K*normalCDF(d2))
	return theta / 365.25
}

// PutTheta returns the daily theta of a put (dPrice/dT per day).
func PutTheta(F, K, r, T, sigma float64) float64 {
	if T <= 0 || sigma <= 0 {
		return 0
	}
	sqrtT := math.Sqrt(T)
	sigmaT := sigma * sqrtT
	d1 := math.FMA(.5, sigmaT, math.Log(F/K)/sigmaT)
	d2 := d1 - sigmaT
	disc := math.Exp(-r * T)
	theta := -F*disc*normalPDF(d1)*sigma/(2*sqrtT) +
		r*disc*(K*normalCDF(-d2)-F*normalCDF(-d1))
	return theta / 365.25
}

// CallDelta returns the delta of a call (dPrice/dF).
func CallDelta(F, K, r, T, sigma float64) float64 {
	if T <= 0 || sigma <= 0 {
		if F > K {
			return math.Exp(-r * T)
		}
		return 0
	}
	sqrtT := math.Sqrt(T)
	sigmaT := sigma * sqrtT
	d1 := math.FMA(.5, sigmaT, math.Log(F/K)/sigmaT)
	return math.Exp(-r*T) * normalCDF(d1)
}

// PutDelta returns the delta of a put (dPrice/dF).
func PutDelta(F, K, r, T, sigma float64) float64 {
	return CallDelta(F, K, r, T, sigma) - math.Exp(-r*T)
}

// normalCDF computes the standard normal cumulative distribution function.
func normalCDF(x float64) float64 {
	return .5 * math.Erfc(-x/math.Sqrt2)
}

// normalPDF computes the standard normal probability density function.
func normalPDF(x float64) float64 {
	return math.Exp(-.5*x*x) / sqrt2Pi
}
