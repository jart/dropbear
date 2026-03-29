package black76

import (
	"dropbear/clocky"
	"fmt"
	"math"
	"testing"
)

const tol = 1e-10

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.10f, want %.10f (diff %.2e)", name, got, want, got-want)
	}
}

// ATM forward: when F == K, call and put prices are equal.
// Parameters: F=100, K=100, r=0.05, E=1.0, sigma=0.20
func TestATMForward(t *testing.T) {
	F, K, r, E, sigma := 100.0, 100.0, 0.05, clocky.Year, 0.20
	call := CallPrice(F, K, r, E, sigma)
	put := PutPrice(F, K, r, E, sigma)
	approx(t, "ATM call", call, 7.5747564442)
	approx(t, "ATM put", put, 7.5747564442)
	approx(t, "call == put", call, put)
}

// Put-call parity for Black-76:
//
//	C - P = e^(-rT) * (F - K)
func TestPutCallParity(t *testing.T) {
	cases := []struct {
		F, K, r float64
		E       clocky.Duration
		sigma   float64
	}{
		{100, 100, 0.05, clocky.Year, 0.20},
		{100, 110, 0.05, clocky.Year, 0.20},
		{100, 90, 0.05, clocky.Year, 0.20},
		{5800, 5700, 0.04, clocky.Hour, 0.15},     // near-expiry, ITM
		{5800, 5900, 0.04, clocky.Hour, 0.15},     // near-expiry, OTM
		{5800, 5800, 0.04, clocky.Year / 2, 0.30}, // 6 months, ATM
	}
	for _, tc := range cases {
		call := CallPrice(tc.F, tc.K, tc.r, tc.E, tc.sigma)
		put := PutPrice(tc.F, tc.K, tc.r, tc.E, tc.sigma)
		parity := math.Exp(-tc.r*Annualize(tc.E)) * (tc.F - tc.K)
		approx(t, "put-call parity", call-put, parity)
	}
}

// OTM call: F=100, K=110, r=0.05, E=0.5, sigma=0.25
func TestOTMCall(t *testing.T) {
	F, K, r, E, sigma := 100.0, 110.0, 0.05, clocky.Year/2, 0.25
	call := CallPrice(F, K, r, E, sigma)
	approx(t, "OTM call", call, 3.3541813989)
}

func TestITMPut(t *testing.T) {
	F, K, r, E, sigma := 100.0, 110.0, 0.05, clocky.Year/2, 0.25
	put := PutPrice(F, K, r, E, sigma)
	call := CallPrice(F, K, r, E, sigma)
	// From parity: P = C - e^(-rT)*(F-K) = C + e^(-rT)*10
	expected := call + math.Exp(-r*Annualize(E))*10
	approx(t, "ITM put", put, expected)
}

// Vega should be positive and identical for calls and puts at the same strike.
func TestVega(t *testing.T) {
	F, K, r, E, sigma := 100.0, 100.0, 0.05, clocky.Year, 0.20
	v := Vega(F, K, r, E, sigma)
	if v <= 0 {
		t.Errorf("vega should be positive, got %f", v)
	}
	approx(t, "ATM vega", v, 37.7477907086)
}

// IV should round-trip: compute price from sigma, then recover sigma from price.
func TestIVRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		F, K, r float64
		E       clocky.Duration
		sigma   float64
		isCall  bool
	}{
		{"ATM call", 100, 100, 0.05, clocky.Year, 0.20, true},
		{"ATM put", 100, 100, 0.05, clocky.Year, 0.20, false},
		{"OTM call", 100, 120, 0.05, clocky.Year, 0.30, true},
		{"OTM put", 100, 80, 0.05, clocky.Year, 0.30, false},
		{"ITM call", 5800, 5700, 0.04, clocky.Year / 2, 0.15, true},
		{"ITM put", 5800, 5900, 0.04, clocky.Year / 2, 0.15, false},
		{"near expiry ATM", 5800, 5800, 0.04, clocky.Hour, 0.20, true},
		{"high vol", 100, 100, 0.05, clocky.Year, 1.50, true},
		{"low vol", 100, 100, 0.05, clocky.Year, 0.05, true},
	}
	for _, tc := range cases {
		var price float64
		if tc.isCall {
			price = CallPrice(tc.F, tc.K, tc.r, tc.E, tc.sigma)
		} else {
			price = PutPrice(tc.F, tc.K, tc.r, tc.E, tc.sigma)
		}
		recovered := IV(tc.F, tc.K, tc.r, tc.E, price, tc.isCall)
		approx(t, tc.name+" IV roundtrip", recovered, tc.sigma)
	}
}

// Edge cases: zero or negative time, zero vol, zero price.
func TestEdgeCases(t *testing.T) {
	// At expiry (T=0): intrinsic value
	approx(t, "call T=0 ITM", CallPrice(110, 100, 0.05, 0, 0.20), 10.0)
	approx(t, "call T=0 OTM", CallPrice(90, 100, 0.05, 0, 0.20), 0.0)
	approx(t, "put T=0 ITM", PutPrice(90, 100, 0.05, 0, 0.20), 10.0)
	approx(t, "put T=0 OTM", PutPrice(110, 100, 0.05, 0, 0.20), 0.0)

	// Vega at expiry
	approx(t, "vega T=0", Vega(100, 100, 0.05, 0, 0.20), 0.0)

	// IV with zero/negative inputs
	approx(t, "IV T=0", IV(100, 100, 0.05, 0, 5.0, true), 0.0)
	approx(t, "IV price=0", IV(100, 100, 0.05, clocky.Year, 0, true), 0.0)
}

// this shows how to get greeks consistent with thinkorswim
// spx 6590.34 0dte 2026-03-19T12:01:53-4000 from thinkorswim ondemand
// 6590 call bid=14.70 ask=14.90 (mid=14.80) (TOS says delta=.51 gamma=.01 theta=-14.46 vega=1.3 iv=.1101)
// 6590 put  bid=14.30 ask=14.50 (mid=14.40) (TOS says delta=-.49 gamma=.01 theta=-14.39 vega=1.3 iv=.1142)
// /SR1 = 96.34 therefore risk free rate is 0.0366
func TestTOSGreeks(t *testing.T) {
	oldDays, oldHours := DaysPerYear, HoursPerDay
	DaysPerYear, HoursPerDay = 252, 6.5
	defer func() { DaysPerYear, HoursPerDay = oldDays, oldHours }()
	now := clocky.MustParseTime("2026-03-19T12:01:53-0400")
	exp := clocky.MustParseTime("2026-03-19T16:00:00-0400")
	E := exp.Sub(now)
	F := 6590.34 // SPX price
	K := 6590.0  // ATM strike
	r := 0.0366  // SR1 risk-free rate (TOS says 3.75%)
	callMid := 14.80
	putMid := 14.50
	callIV := IV(F, K, r, E, callMid, true)
	putIV := IV(F, K, r, E, putMid, false)
	callDelta, callTheta, callGamma, callVega := CallGreeks(F, K, r, E, callIV)
	putDelta, putTheta, putGamma, putVega := PutGreeks(F, K, r, E, putIV)
	callVega /= 100
	putVega /= 100
	wantCall := "delta=0.5048 gamma=0.0109 theta=-11.9777 vega=1.2939 iv=0.1131"
	gotCall := fmt.Sprintf("delta=%.4f gamma=%.4f theta=%.4f vega=%.4f iv=%.4f", callDelta, callGamma, callTheta, callVega, callIV)
	if gotCall != wantCall {
		t.Errorf("got %s, want %s", gotCall, wantCall)
	}
	wantPut := "delta=-0.4952 gamma=0.0108 theta=-12.0105 vega=1.2939 iv=0.1134"
	gotPut := fmt.Sprintf("delta=%.4f gamma=%.4f theta=%.4f vega=%.4f iv=%.4f", putDelta, putGamma, putTheta, putVega, putIV)
	if gotPut != wantPut {
		t.Errorf("got %s, want %s", gotPut, wantPut)
	}
}

func BenchmarkCallGreeks(b *testing.B) {
	for b.Loop() {
		CallGreeks(5800, 5700, 0.04, clocky.Day, 0.20)
	}
}

func BenchmarkPutGreeks(b *testing.B) {
	for b.Loop() {
		PutGreeks(5800, 5700, 0.04, clocky.Day, 0.20)
	}
}

func BenchmarkIV(b *testing.B) {
	price := CallPrice(5800, 5700, 0.04, clocky.Year/2, 0.20)
	for b.Loop() {
		IV(5800, 5700, 0.04, clocky.Year/2, price, true)
	}
}
