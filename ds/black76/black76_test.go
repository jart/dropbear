package black76

import (
	"math"
	"testing"
)

const tol = 1e-6

func approx(t *testing.T, name string, got, want, eps float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s = %.10f, want %.10f (diff %.2e)", name, got, want, got-want)
	}
}

// ATM forward: when F == K, call and put prices are equal.
// Parameters: F=100, K=100, r=0.05, T=1.0, sigma=0.20
func TestATMForward(t *testing.T) {
	F, K, r, T, sigma := 100.0, 100.0, 0.05, 1.0, 0.20
	call := Call(F, K, r, T, sigma)
	put := Put(F, K, r, T, sigma)
	approx(t, "ATM call", call, 7.5770821464, 1e-8)
	approx(t, "ATM put", put, 7.5770821464, 1e-8)
	approx(t, "call == put", call, put, tol)
}

// Put-call parity for Black-76:
//   C - P = e^(-rT) * (F - K)
func TestPutCallParity(t *testing.T) {
	cases := []struct {
		F, K, r, T, sigma float64
	}{
		{100, 100, 0.05, 1.0, 0.20},
		{100, 110, 0.05, 1.0, 0.20},
		{100, 90, 0.05, 1.0, 0.20},
		{5800, 5700, 0.04, 0.001, 0.15}, // near-expiry, ITM
		{5800, 5900, 0.04, 0.001, 0.15}, // near-expiry, OTM
		{5800, 5800, 0.04, 0.5, 0.30},   // 6 months, ATM
	}
	for _, tc := range cases {
		call := Call(tc.F, tc.K, tc.r, tc.T, tc.sigma)
		put := Put(tc.F, tc.K, tc.r, tc.T, tc.sigma)
		parity := math.Exp(-tc.r*tc.T) * (tc.F - tc.K)
		approx(t, "put-call parity", call-put, parity, 1e-8)
	}
}

// OTM call: F=100, K=110, r=0.05, T=0.5, sigma=0.25
func TestOTMCall(t *testing.T) {
	F, K, r, T, sigma := 100.0, 110.0, 0.05, 0.5, 0.25
	call := Call(F, K, r, T, sigma)
	approx(t, "OTM call", call, 3.3562508126, 1e-8)
}

func TestITMPut(t *testing.T) {
	F, K, r, T, sigma := 100.0, 110.0, 0.05, 0.5, 0.25
	put := Put(F, K, r, T, sigma)
	call := Call(F, K, r, T, sigma)
	// From parity: P = C - e^(-rT)*(F-K) = C + e^(-rT)*10
	expected := call + math.Exp(-r*T)*10
	approx(t, "ITM put", put, expected, tol)
}

// Vega should be positive and identical for calls and puts at the same strike.
func TestVega(t *testing.T) {
	F, K, r, T, sigma := 100.0, 100.0, 0.05, 1.0, 0.20
	v := Vega(F, K, r, T, sigma)
	if v <= 0 {
		t.Errorf("vega should be positive, got %f", v)
	}
	// Vega = F * e^(-rT) * N'(d1) * sqrt(T)
	// For ATM (F=K): d1 = sigma*sqrt(T)/2 = 0.10
	// N'(0.10) = 0.39695
	// Vega = 100 * e^(-0.05) * 0.39695 * 1.0 = 37.76
	approx(t, "ATM vega", v, 37.76, 0.1)
}

// IV should round-trip: compute price from sigma, then recover sigma from price.
func TestIVRoundTrip(t *testing.T) {
	cases := []struct {
		name                string
		F, K, r, T, sigma   float64
		isCall               bool
	}{
		{"ATM call", 100, 100, 0.05, 1.0, 0.20, true},
		{"ATM put", 100, 100, 0.05, 1.0, 0.20, false},
		{"OTM call", 100, 120, 0.05, 1.0, 0.30, true},
		{"OTM put", 100, 80, 0.05, 1.0, 0.30, false},
		{"ITM call", 5800, 5700, 0.04, 0.5, 0.15, true},
		{"ITM put", 5800, 5900, 0.04, 0.5, 0.15, false},
		{"near expiry ATM", 5800, 5800, 0.04, 0.001, 0.20, true},
		{"high vol", 100, 100, 0.05, 1.0, 1.50, true},
		{"low vol", 100, 100, 0.05, 1.0, 0.05, true},
	}
	for _, tc := range cases {
		var price float64
		if tc.isCall {
			price = Call(tc.F, tc.K, tc.r, tc.T, tc.sigma)
		} else {
			price = Put(tc.F, tc.K, tc.r, tc.T, tc.sigma)
		}
		recovered := IV(tc.F, tc.K, tc.r, tc.T, price, tc.isCall)
		approx(t, tc.name+" IV roundtrip", recovered, tc.sigma, 1e-6)
	}
}

// Edge cases: zero or negative time, zero vol, zero price.
func TestEdgeCases(t *testing.T) {
	// At expiry (T=0): intrinsic value
	approx(t, "call T=0 ITM", Call(110, 100, 0.05, 0, 0.20), 10.0, tol)
	approx(t, "call T=0 OTM", Call(90, 100, 0.05, 0, 0.20), 0.0, tol)
	approx(t, "put T=0 ITM", Put(90, 100, 0.05, 0, 0.20), 10.0, tol)
	approx(t, "put T=0 OTM", Put(110, 100, 0.05, 0, 0.20), 0.0, tol)

	// Zero vol: intrinsic value discounted
	approx(t, "call sigma=0 ITM", Call(110, 100, 0.05, 1.0, 0), 10*math.Exp(-0.05), tol)
	approx(t, "call sigma=0 OTM", Call(90, 100, 0.05, 1.0, 0), 0.0, tol)

	// Vega at expiry
	approx(t, "vega T=0", Vega(100, 100, 0.05, 0, 0.20), 0.0, tol)

	// IV with zero/negative inputs
	approx(t, "IV T=0", IV(100, 100, 0.05, 0, 5.0, true), 0.0, tol)
	approx(t, "IV price=0", IV(100, 100, 0.05, 1.0, 0, true), 0.0, tol)
}

// Benchmark Black-76 call pricing.
func BenchmarkCall(b *testing.B) {
	for b.Loop() {
		Call(5800, 5700, 0.04, 0.001, 0.20)
	}
}

// Benchmark implied vol solver.
func BenchmarkIV(b *testing.B) {
	price := Call(5800, 5700, 0.04, 0.5, 0.20)
	for b.Loop() {
		IV(5800, 5700, 0.04, 0.5, price, true)
	}
}
