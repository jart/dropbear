package indicators

import (
	"dropbear/decimal"
	"fmt"
	"math"
	"testing"
)

func decFromFloat(f float64) decimal.Decimal {
	return decimal.Parse(fmt.Sprintf("%.9f", f))
}

func TestSwissArmyKnifeGauss(t *testing.T) {
	// Gaussian filter should smooth price data
	g := NewGaussFilter(10)

	// Feed constant price - output should converge to that price
	for i := 0; i < 100; i++ {
		g.Add(decimal.FromInt(100))
	}
	if g.Value.Sub(decimal.FromInt(100)).Abs().Cmp(decimal.Parse("0.01")) > 0 {
		t.Errorf("Gauss filter didn't converge to constant: got %s", g.Value)
	}
}

func TestSwissArmyKnifeButter(t *testing.T) {
	// Butterworth should also smooth to constant
	b := NewButterFilter(10)

	for i := 0; i < 100; i++ {
		b.Add(decimal.FromInt(100))
	}
	if b.Value.Sub(decimal.FromInt(100)).Abs().Cmp(decimal.Parse("0.01")) > 0 {
		t.Errorf("Butter filter didn't converge to constant: got %s", b.Value)
	}
}

func TestSwissArmyKnifeHighPass(t *testing.T) {
	// High pass removes DC component - constant input should go to zero
	h := NewHighPassFilter(10)

	for i := 0; i < 100; i++ {
		h.Add(decimal.FromInt(100))
	}
	// Should be near zero since there's no high frequency content
	if h.Value.Abs().Cmp(decimal.Parse("1")) > 0 {
		t.Errorf("HighPass filter should remove DC: got %s", h.Value)
	}
}

func TestSwissArmyKnifeTwoPoleHighPass(t *testing.T) {
	// Two-pole high pass also removes DC
	h := NewTwoPoleHighPassFilter(10)

	for i := 0; i < 100; i++ {
		h.Add(decimal.FromInt(100))
	}
	if h.Value.Abs().Cmp(decimal.Parse("1")) > 0 {
		t.Errorf("TwoPoleHighPass filter should remove DC: got %s", h.Value)
	}
}

func TestSwissArmyKnifeOscillation(t *testing.T) {
	// Test with oscillating signal - high pass should preserve it
	h := NewHighPassFilter(20)
	g := NewGaussFilter(20)

	for i := 0; i < 200; i++ {
		// Oscillate between 90 and 110 with period 10
		price := decFromFloat(100 + 10*math.Sin(float64(i)*2*math.Pi/10))
		h.Add(price)
		g.Add(price)
	}

	// High pass should have significant output for this oscillation
	// Gauss should smooth it significantly
	t.Logf("HighPass output for oscillation: %s", h.Value)
	t.Logf("Gauss output for oscillation: %s", g.Value)
}

func TestSwissArmyKnifeTrendWithNoise(t *testing.T) {
	// Uptrend with noise - Gauss should smooth, HighPass should show only noise
	g := NewGaussFilter(20)
	h := NewHighPassFilter(20)

	for i := 0; i < 200; i++ {
		// Linear trend + small oscillation
		trend := float64(100 + i)
		noise := 2 * math.Sin(float64(i)*2*math.Pi/5)
		price := decFromFloat(trend + noise)
		g.Add(price)
		h.Add(price)
	}

	// Gauss should be close to the trend (around 299 at i=199)
	if g.Value.Cmp(decimal.FromInt(250)) < 0 || g.Value.Cmp(decimal.FromInt(320)) > 0 {
		t.Errorf("Gauss should follow trend: got %s, expected ~299", g.Value)
	}

	t.Logf("Gauss (smoothed): %s", g.Value)
	t.Logf("HighPass (detrended): %s", h.Value)
}

func TestSwissArmyKnifeBandPass(t *testing.T) {
	// Band pass should isolate specific frequencies
	bp := NewBandPassFilter(20, 0.3)

	for i := 0; i < 200; i++ {
		price := decFromFloat(100 + 10*math.Sin(float64(i)*2*math.Pi/20))
		bp.Add(price)
	}

	// Should have significant output for matching frequency
	if !bp.IsReady() {
		t.Error("BandPass should be ready")
	}
	t.Logf("BandPass output for matching frequency: %s", bp.Value)
}

func BenchmarkSwissArmyKnifeGauss(b *testing.B) {
	g := NewGaussFilter(50)
	price := decimal.FromInt(100)
	for i := 0; i < b.N; i++ {
		g.Add(price)
	}
}

func BenchmarkSwissArmyKnifeButter(b *testing.B) {
	f := NewButterFilter(50)
	price := decimal.FromInt(100)
	for i := 0; i < b.N; i++ {
		f.Add(price)
	}
}
