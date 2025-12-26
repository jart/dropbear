package indicators

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"math/rand"
	"testing"
)

func TestIntensityBasic(t *testing.T) {
	ti := NewIntensity(5 * clocky.Minute)
	ti.SetMidPrice(decimal.FromInt(100))

	// simulate trades with exponentially decaying intensity from mid
	// more trades near mid, fewer far from mid
	now := clocky.Time(0)
	rng := rand.New(rand.NewSource(42))

	for range 1000 {
		// generate distance from mid with exponential distribution
		// this should give us κ ≈ 10 (1/0.1 decay rate)
		delta := rng.ExpFloat64() / 10 // mean distance = 0.1 (10%)
		price := decimal.FromInt(99).Mul(decimal.One.Add(decimal.FromFloat64(delta)))

		volume := decimal.One
		ti.AddTrade(now, price, volume)
		now = now.Add(clocky.Millisecond * 100)
	}

	if !ti.IsReady() {
		t.Error("should be ready after 1000 trades")
	}

	t.Logf("Alpha: %s", ti.Alpha)
	t.Logf("Kappa: %s", ti.Kappa)
	t.Logf("OptimalSpread: %s (%sbps)", ti.OptimalSpread(), ti.OptimalSpread().BPS())
	t.Logf("TotalVolume: %.1f", ti.totalVolume())

	// kappa should be positive for exponentially distributed trades
	if ti.Kappa.IsZero() || ti.Kappa.IsNegative() {
		t.Error("kappa should be positive")
	}
}

func TestIntensityTightLiquidity(t *testing.T) {
	ti := NewIntensity(5 * clocky.Minute)
	ti.SetMidPrice(decimal.FromInt(100))

	// simulate tight liquidity: all trades very near mid-price
	now := clocky.Time(0)
	rng := rand.New(rand.NewSource(42))

	for range 500 {
		// very small deviations from mid (high kappa)
		delta := rng.ExpFloat64() / 100 // mean distance = 0.01 (1%)
		price := decimal.FromInt(100).Mul(decimal.One.Add(decimal.FromFloat64(delta)))
		volume := decimal.One
		ti.AddTrade(now, price, volume)
		now = now.Add(clocky.Millisecond * 100)
	}

	t.Logf("Tight liquidity - Kappa: %s", ti.Kappa)
}

func TestIntensityDispersedLiquidity(t *testing.T) {
	ti := NewIntensity(5 * clocky.Minute)
	ti.SetMidPrice(decimal.FromInt(100))

	// simulate dispersed liquidity: trades spread across price levels
	now := clocky.Time(0)
	rng := rand.New(rand.NewSource(42))

	for range 500 {
		// larger deviations from mid (low kappa)
		delta := rng.ExpFloat64() / 2 // mean distance = 0.5 (50%)
		price := decimal.FromInt(100).Mul(decimal.One.Add(decimal.FromFloat64(delta)))
		volume := decimal.One
		ti.AddTrade(now, price, volume)
		now = now.Add(clocky.Millisecond * 100)
	}

	t.Logf("Dispersed liquidity - Kappa: %s", ti.Kappa)
}

func TestIntensityExponentialDecay(t *testing.T) {
	ti := NewIntensity(clocky.Minute) // 1 minute half-life

	ti.SetMidPrice(decimal.FromInt(100))
	now := clocky.Time(0)

	// add trades for 5 half-lives
	for i := range 500 {
		delta := float64(i%10) * 0.001
		price := decimal.FromInt(100).Mul(decimal.One.Add(decimal.FromFloat64(delta)))
		ti.AddTrade(now, price, decimal.One)
		now = now.Add(clocky.Second * 2) // 2s per trade = 1000s total = 16.7 minutes
	}

	// after 5+ half-lives, total volume should be much less than 50000 (500 trades × $100)
	// steady state volume ≈ trades_per_halflife × price = (60s / 2s) × $100 = $3000
	// binVolume stores USD value, not raw quantity
	vol := ti.totalVolume()
	if vol > 10000 {
		t.Errorf("expected volume < 10000 due to decay, got %.1f", vol)
	}
	t.Logf("Volume after 5 half-lives: %.1f", vol)
}

func TestIntensityCompareKappas(t *testing.T) {
	// verify that tighter liquidity produces higher kappa
	tightTI := NewIntensity(5 * clocky.Minute)
	tightTI.SetMidPrice(decimal.FromInt(100))

	dispersedTI := NewIntensity(5 * clocky.Minute)
	dispersedTI.SetMidPrice(decimal.FromInt(100))

	now := clocky.Time(0)
	rng := rand.New(rand.NewSource(42))

	for range 500 {
		// tight: small deltas
		tightDelta := rng.ExpFloat64() / 100
		tightPrice := decimal.FromInt(100).Mul(decimal.One.Add(decimal.FromFloat64(tightDelta)))
		tightTI.AddTrade(now, tightPrice, decimal.One)

		// dispersed: larger deltas
		dispersedDelta := rng.ExpFloat64() / 5
		dispersedPrice := decimal.FromInt(100).Mul(decimal.One.Add(decimal.FromFloat64(dispersedDelta)))
		dispersedTI.AddTrade(now, dispersedPrice, decimal.One)

		now = now.Add(clocky.Millisecond * 100)
	}

	t.Logf("Tight kappa: %s", tightTI.Kappa)
	t.Logf("Dispersed kappa: %s", dispersedTI.Kappa)

	if tightTI.Kappa.Cmp(dispersedTI.Kappa) <= 0 {
		t.Error("tight liquidity should have higher kappa than dispersed")
	}
}

func BenchmarkIntensity(b *testing.B) {
	ti := NewIntensity(5 * clocky.Minute)
	ti.SetMidPrice(decimal.FromInt(100))

	// pre-warm
	now := clocky.Time(0)
	for i := range 100 {
		delta := float64(i%10) * 0.001
		price := decimal.FromInt(100).Mul(decimal.One.Add(decimal.FromFloat64(delta)))
		ti.AddTrade(now, price, decimal.One)
		now = now.Add(clocky.Millisecond * 100)
	}

	for i := 0; b.Loop(); i++ {
		delta := float64(i%10) * 0.001
		price := decimal.FromInt(100).Mul(decimal.One.Add(decimal.FromFloat64(delta)))
		ti.AddTrade(now, price, decimal.One)
		now = now.Add(clocky.Millisecond * 100)
		_ = ti.IsReady()
	}
}

func (ti *Intensity) totalVolume() float64 {
	total := 0.0
	for i := range numBins {
		total += ti.binVolume[i]
	}
	return total
}

func (ti *Intensity) filledBins() int {
	count := 0
	for i := range numBins {
		if ti.binVolume[i] > minVolumeBin {
			count++
		}
	}
	return count
}
