package main

import (
	"dropbear/netty"
	"math"
	"math/rand"
	"testing"
)

func init() {
	netty.SetOffline()
}

// TestGrangerCausalitySynthetic verifies that the Granger test detects a known
// causal relationship in synthetic data. We construct y(t) = 0.5*x(t-1) + noise
// so x should Granger-cause y.
func TestGrangerCausalitySynthetic(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	n := 500

	x := make([]float64, n)
	y := make([]float64, n)

	// x is iid noise
	for i := range x {
		x[i] = rng.NormFloat64() * 0.01
	}

	// y depends on lagged x
	for i := 1; i < n; i++ {
		y[i] = 0.5*x[i-1] + rng.NormFloat64()*0.01
	}

	fstat, pval, bestLag, _ := grangerCausality(x, y, 5)
	t.Logf("x -> y: F=%.2f, p=%.2e, lag=%d", fstat, pval, bestLag)

	if pval >= 0.01 {
		t.Errorf("expected x to Granger-cause y (p < 0.01), got p=%.4f", pval)
	}
	if bestLag != 1 {
		t.Errorf("expected best lag = 1, got %d", bestLag)
	}

	// y should NOT Granger-cause x (reverse direction)
	fstat2, pval2, _, _ := grangerCausality(y, x, 5)
	t.Logf("y -> x: F=%.2f, p=%.2e", fstat2, pval2)

	if pval2 < 0.01 {
		t.Errorf("expected y NOT to Granger-cause x, but got p=%.4f", pval2)
	}
}

// TestGrangerCausalityIndependent verifies that independent series don't show
// spurious Granger causality.
func TestGrangerCausalityIndependent(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	n := 500

	x := make([]float64, n)
	y := make([]float64, n)
	for i := range x {
		x[i] = rng.NormFloat64() * 0.01
		y[i] = rng.NormFloat64() * 0.01
	}

	_, pval, _, _ := grangerCausality(x, y, 5)
	if pval < 0.01 {
		t.Errorf("independent series should not be significant at p=0.01, got p=%.4f", pval)
	}
}

// TestGrangerCausalityLag2 tests detection of a lag-2 relationship.
func TestGrangerCausalityLag2(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	n := 500

	x := make([]float64, n)
	y := make([]float64, n)

	for i := range x {
		x[i] = rng.NormFloat64() * 0.01
	}

	// y depends on x at lag 2
	for i := 2; i < n; i++ {
		y[i] = 0.4*x[i-2] + rng.NormFloat64()*0.01
	}

	fstat, pval, bestLag, _ := grangerCausality(x, y, 5)
	t.Logf("x -> y (lag 2): F=%.2f, p=%.2e, bestLag=%d", fstat, pval, bestLag)

	if pval >= 0.01 {
		t.Errorf("expected significance at p < 0.01, got p=%.4f", pval)
	}
	// bestLag should be 2 or higher (anything >= 2 can capture the relationship)
	if bestLag < 2 {
		t.Errorf("expected best lag >= 2, got %d", bestLag)
	}
}

// TestFDistPValue checks the F-distribution p-value calculation against known values.
func TestFDistPValue(t *testing.T) {
	tests := []struct {
		f        float64
		d1, d2   int
		expected float64
		tol      float64
	}{
		// F(1, 100) = 3.94 => p ≈ 0.05
		{3.94, 1, 100, 0.05, 0.005},
		// F(1, 100) = 6.90 => p ≈ 0.01
		{6.90, 1, 100, 0.01, 0.005},
		// F(5, 200) = 2.26 => p ≈ 0.05
		{2.26, 5, 200, 0.05, 0.005},
		// F = 0 => p = 1
		{0, 5, 200, 1.0, 0.001},
	}

	for _, tt := range tests {
		p := fDistPValue(tt.f, tt.d1, tt.d2)
		if math.Abs(p-tt.expected) > tt.tol {
			t.Errorf("fDistPValue(%g, %d, %d) = %g, want %g (±%g)",
				tt.f, tt.d1, tt.d2, p, tt.expected, tt.tol)
		}
	}
}

// TestCholeskySolve verifies the Cholesky solver on a known system.
func TestCholeskySolve(t *testing.T) {
	// Solve: [[4,2],[2,3]] * x = [1, 2]
	// Solution: x = [-0.125, 0.75]
	A := []float64{4, 2, 2, 3}
	b := []float64{1, 2}
	x := choleskySolve(A, b, 2)
	if x == nil {
		t.Fatal("choleskySolve returned nil")
	}
	if math.Abs(x[0]-(-0.125)) > 1e-10 || math.Abs(x[1]-0.75) > 1e-10 {
		t.Errorf("expected [-0.125, 0.75], got [%g, %g]", x[0], x[1])
	}
}

// TestRegularizedBeta checks the incomplete beta function.
func TestRegularizedBeta(t *testing.T) {
	tests := []struct {
		x, a, b  float64
		expected float64
		tol      float64
	}{
		{0.5, 1, 1, 0.5, 1e-10},
		{0, 2, 3, 0, 1e-10},
		{1, 2, 3, 1, 1e-10},
		{0.5, 2, 3, 0.6875, 1e-6},
	}

	for _, tt := range tests {
		got := regularizedBeta(tt.x, tt.a, tt.b)
		if math.Abs(got-tt.expected) > tt.tol {
			t.Errorf("regularizedBeta(%g, %g, %g) = %g, want %g",
				tt.x, tt.a, tt.b, got, tt.expected)
		}
	}
}

func BenchmarkGrangerCausality(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	n := 252
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range x {
		x[i] = rng.NormFloat64() * 0.01
	}
	for i := 1; i < n; i++ {
		y[i] = 0.3*x[i-1] + rng.NormFloat64()*0.01
	}

	b.ResetTimer()
	for range b.N {
		grangerCausality(x, y, 5)
	}
}
