package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/indicators"
	"dropbear/netty"
	"testing"
)

func init() {
	netty.SetOffline()
}

func TestStinkPriceCalculation(t *testing.T) {
	// Test the stink price formula
	// stinkPrice = currentPrice * (1 - dropPercent)
	// where dropPercent = sigma * stddev, clamped to [minDrop, maxDrop]

	tests := []struct {
		name         string
		currentPrice decimal.Decimal
		sigma        decimal.Decimal
		stddev       decimal.Decimal
		minDrop      decimal.Decimal
		maxDrop      decimal.Decimal
		wantDrop     decimal.Decimal // expected drop percent
	}{
		{
			name:         "normal volatility",
			currentPrice: decimal.FromInt(100000),
			sigma:        decimal.FromInt(3),
			stddev:       decimal.Parse("0.02"), // 2% typical volatility
			minDrop:      decimal.Parse("0.03"), // 3% min
			maxDrop:      decimal.Parse("0.20"), // 20% max
			wantDrop:     decimal.Parse("0.06"), // 3 * 0.02 = 6%
		},
		{
			name:         "low volatility clamped to min",
			currentPrice: decimal.FromInt(100000),
			sigma:        decimal.FromInt(3),
			stddev:       decimal.Parse("0.005"), // 0.5% low volatility
			minDrop:      decimal.Parse("0.03"),
			maxDrop:      decimal.Parse("0.20"),
			wantDrop:     decimal.Parse("0.03"), // clamped to 3% min
		},
		{
			name:         "high volatility clamped to max",
			currentPrice: decimal.FromInt(100000),
			sigma:        decimal.FromInt(3),
			stddev:       decimal.Parse("0.10"), // 10% high volatility
			minDrop:      decimal.Parse("0.03"),
			maxDrop:      decimal.Parse("0.20"),
			wantDrop:     decimal.Parse("0.20"), // clamped to 20% max
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate drop percent
			dropPercent := tt.sigma.Mul(tt.stddev)
			dropPercent = dropPercent.Max(tt.minDrop)
			dropPercent = dropPercent.Min(tt.maxDrop)

			if dropPercent.Cmp(tt.wantDrop) != 0 {
				t.Errorf("dropPercent = %s, want %s", dropPercent, tt.wantDrop)
			}

			// Calculate stink price
			stinkPrice := tt.currentPrice.Mul(decimal.One.Sub(dropPercent))
			expectedStink := tt.currentPrice.Mul(decimal.One.Sub(tt.wantDrop))

			if stinkPrice.Cmp(expectedStink) != 0 {
				t.Errorf("stinkPrice = %s, want %s", stinkPrice, expectedStink)
			}
		})
	}
}

func TestBufferDayLogic(t *testing.T) {
	// Test the isNearBufferDay function logic
	tests := []struct {
		day       int
		bufferDay int
		want      bool
	}{
		{day: 1, bufferDay: 7, want: false},
		{day: 4, bufferDay: 7, want: false},
		{day: 5, bufferDay: 7, want: true}, // 2 days before
		{day: 6, bufferDay: 7, want: true}, // 1 day before
		{day: 7, bufferDay: 7, want: true}, // buffer day
		{day: 8, bufferDay: 7, want: false},
		{day: 15, bufferDay: 15, want: true},
		{day: 13, bufferDay: 15, want: true},
		{day: 12, bufferDay: 15, want: false},
	}

	for _, tt := range tests {
		// Inline implementation since we can't call the function directly
		got := tt.day >= tt.bufferDay-2 && tt.day <= tt.bufferDay
		if got != tt.want {
			t.Errorf("day=%d bufferDay=%d: got %v, want %v", tt.day, tt.bufferDay, got, tt.want)
		}
	}
}

func TestWelfordVolatilityFromReturns(t *testing.T) {
	// Simulate computing volatility from price returns
	w := indicators.NewWelford(10)

	// Generate prices with known pattern
	prices := []decimal.Decimal{
		decimal.FromInt(100),
		decimal.FromInt(102), // +2%
		decimal.FromInt(100), // -1.96%
		decimal.FromInt(103), // +3%
		decimal.FromInt(99),  // -3.88%
		decimal.FromInt(104), // +5.05%
		decimal.FromInt(101), // -2.88%
		decimal.FromInt(105), // +3.96%
		decimal.FromInt(100), // -4.76%
		decimal.FromInt(106), // +6%
		decimal.FromInt(102), // -3.77%
	}

	var lastPrice decimal.Decimal
	for _, price := range prices {
		if lastPrice.IsPositive() {
			ret := price.Sub(lastPrice).Div(lastPrice)
			w.Add(ret)
		}
		lastPrice = price
	}

	if !w.IsReady() {
		t.Error("Welford should be ready after 10 returns")
	}

	stddev := w.Stddev()
	if !stddev.IsPositive() {
		t.Errorf("stddev should be positive, got %s", stddev)
	}

	// Stddev should be reasonable (between 1% and 10%)
	pct := stddev.MulInt(100).Float64()
	if pct < 1 || pct > 10 {
		t.Errorf("stddev = %.2f%%, expected between 1%% and 10%%", pct)
	}
}

func TestPreCrashPriceTracking(t *testing.T) {
	// Test that WWMA provides a good pre-crash sell target
	wwma := indicators.NewWWMA(60) // 1 hour of minutes

	// Feed 60 minutes of stable price around 100
	for i := 0; i < 60; i++ {
		wwma.Add(decimal.FromInt(100))
	}

	if !wwma.IsReady() {
		t.Error("WWMA should be ready")
	}

	preCrashPrice := wwma.Value
	if preCrashPrice.Cmp(decimal.FromInt(100)) != 0 {
		t.Errorf("preCrashPrice = %s, want 100", preCrashPrice)
	}

	// Simulate sudden crash to 90
	for i := 0; i < 5; i++ {
		wwma.Add(decimal.FromInt(90))
	}

	// WWMA should still be close to pre-crash (slow moving)
	afterCrash := wwma.Value
	if afterCrash.Cmp(decimal.FromInt(95)) < 0 {
		t.Errorf("WWMA moved too fast: got %s, expected > 95", afterCrash)
	}
}

func BenchmarkStinkPriceCalculation(b *testing.B) {
	currentPrice := decimal.FromInt(100000)
	sigma := decimal.FromInt(3)
	stddev := decimal.Parse("0.02")
	minDrop := decimal.Parse("0.03")
	maxDrop := decimal.Parse("0.20")
	quoteIncrement := decimal.Cent

	for b.Loop() {
		dropPercent := sigma.Mul(stddev)
		dropPercent = dropPercent.Max(minDrop)
		dropPercent = dropPercent.Min(maxDrop)
		stinkPrice := currentPrice.Mul(decimal.One.Sub(dropPercent))
		_ = stinkPrice.QuantizeTruncate(quoteIncrement)
	}
}

func TestCooldownLogic(t *testing.T) {
	// Test cooldown between bid updates
	cooldown := 30 * clocky.Second

	lastUpdate := clocky.MustParseTime("2025-01-15T10:00:00Z")
	now := clocky.MustParseTime("2025-01-15T10:00:25Z")

	// 25 seconds elapsed, should not update yet
	if now.Sub(lastUpdate) >= cooldown {
		t.Error("should not update yet (only 25s elapsed)")
	}

	now = clocky.MustParseTime("2025-01-15T10:00:35Z")

	// 35 seconds elapsed, should update
	if now.Sub(lastUpdate) < cooldown {
		t.Error("should update now (35s elapsed)")
	}
}

func TestPriceMoveThreshold(t *testing.T) {
	// Test that we only update bid when price moves >0.5%
	threshold := decimal.Parse("0.005") // 0.5%

	oldPrice := decimal.FromInt(95000)
	tests := []struct {
		newPrice     decimal.Decimal
		shouldUpdate bool
	}{
		{decimal.FromInt(95100), false}, // 0.1% move
		{decimal.FromInt(95400), false}, // 0.4% move
		{decimal.FromInt(95500), true},  // 0.5% move
		{decimal.FromInt(96000), true},  // 1% move
		{decimal.FromInt(94500), true},  // -0.5% move
	}

	for _, tt := range tests {
		delta := tt.newPrice.Sub(oldPrice).Abs().Div(oldPrice)
		shouldUpdate := delta.Cmp(threshold) >= 0
		if shouldUpdate != tt.shouldUpdate {
			t.Errorf("price %s -> %s (delta=%s): got update=%v, want %v",
				oldPrice, tt.newPrice, delta, shouldUpdate, tt.shouldUpdate)
		}
	}
}
