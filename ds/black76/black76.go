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

// Call computes all greeks at once cheaply for call options.
// - F is the forward price of the underlying future
// - K is the option strike
// - r is the risk free rate (e.g. 0.05 for 5%)
// - T is the time to expiration in years
// - sigma is the volatility
func Call(F, K, r, T, sigma float64) (price, delta, theta, gamma, vega float64) {
	disc := math.Exp(-r * T)
	if T <= 0 || sigma <= 0 {
		fmk := F - K
		price = math.Max(fmk, 0) * disc
		if fmk > 0 {
			delta = disc
		} else {
			delta = 0
		}
		theta = 0
		gamma = 0
		vega = 0
		return
	}
	sqrtT := math.Sqrt(T)
	sigmaT := sigma * sqrtT
	d1 := math.FMA(.5, sigmaT, math.Log(F/K)/sigmaT)
	d2 := d1 - sigmaT
	Nd1 := normalCDF(d1)
	Nd2 := normalCDF(d2)
	npd1 := normalPDF(d1)
	FNd1 := F * Nd1
	KNd2 := K * Nd2
	callpv := FNd1 - KNd2
	decay := -F * npd1 * sigma / (2 * sqrtT)
	price = disc * callpv
	delta = disc * Nd1
	theta = disc * math.FMA(r, callpv, decay) / 365.25
	gamma = disc * npd1 / (F * sigmaT)
	vega = F * disc * npd1 * sqrtT
	return
}

// Put computes all greeks at once cheaply for put options.
// - F is the forward price of the underlying future
// - K is the option strike
// - r is the risk free rate (e.g. 0.05 for 5%)
// - T is the time to expiration in years
// - sigma is the volatility
func Put(F, K, r, T, sigma float64) (price, delta, theta, gamma, vega float64) {
	disc := math.Exp(-r * T)
	if T <= 0 || sigma <= 0 {
		fmk := F - K
		price = math.Max(-fmk, 0) * disc
		if fmk > 0 {
			delta = disc - disc
		} else {
			delta = -disc
		}
		theta = 0
		gamma = 0
		vega = 0
		return
	}
	sqrtT := math.Sqrt(T)
	sigmaT := sigma * sqrtT
	d1 := math.FMA(.5, sigmaT, math.Log(F/K)/sigmaT)
	d2 := d1 - sigmaT
	Nd1 := normalCDF(d1)
	Nd2 := normalCDF(d2)
	npd1 := normalPDF(d1)
	FNd1 := F * Nd1
	KNd2 := K * Nd2
	putpv := K - KNd2 - F + FNd1
	decay := -F * npd1 * sigma / (2 * sqrtT)
	price = disc * putpv
	delta = disc*Nd1 - disc
	theta = disc * math.FMA(r, putpv, decay) / 365.25
	gamma = disc * npd1 / (F * sigmaT)
	vega = F * disc * npd1 * sqrtT
	return
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
	// bisection to find initial bounds [lo, hi] bracketing the solution
	lo, hi := 0.001, 5.0
	for hi < 100 {
		price := 0.0
		if isCall {
			price, _, _, _, _ = Call(F, K, r, T, hi)
		} else {
			price, _, _, _, _ = Put(F, K, r, T, hi)
		}
		if price >= marketPrice {
			break
		}
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

// normalCDF computes the standard normal cumulative distribution function.
func normalCDF(x float64) float64 {
	return .5 * math.Erfc(-x/math.Sqrt2)
}

// normalPDF computes the standard normal probability density function.
func normalPDF(x float64) float64 {
	return math.Exp(-.5*x*x) / sqrt2Pi
}
