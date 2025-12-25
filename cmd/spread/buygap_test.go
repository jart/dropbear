package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"testing"
)

func TestDecayModeString(t *testing.T) {
	tests := []struct {
		mode DecayMode
		want string
	}{
		{DecayExponential, "exponential"},
		{DecayLinear, "linear"},
		{DecayNone, "none"},
	}

	for _, tt := range tests {
		got := tt.mode.String()
		if got != tt.want {
			t.Errorf("DecayMode(%d).String() = %s, want %s", tt.mode, got, tt.want)
		}
	}
}

func TestParseDecayMode(t *testing.T) {
	tests := []struct {
		input string
		want  DecayMode
	}{
		{"exponential", DecayExponential},
		{"linear", DecayLinear},
		{"none", DecayNone},
		{"invalid", DecayExponential}, // defaults to exponential
		{"", DecayExponential},
	}

	for _, tt := range tests {
		got := ParseDecayMode(tt.input)
		if got != tt.want {
			t.Errorf("ParseDecayMode(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestCalculateDecayFactor_Linear(t *testing.T) {
	tests := []struct {
		name           string
		timeSinceSell  clocky.Duration
		decayPeriod    clocky.Duration
		inventoryRatio decimal.Decimal
		want           decimal.Decimal
	}{
		{
			name:           "zero time",
			timeSinceSell:  0,
			decayPeriod:    30 * clocky.Second,
			inventoryRatio: decimal.Zero,
			want:           decimal.One, // full protection
		},
		{
			name:           "half period",
			timeSinceSell:  15 * clocky.Second,
			decayPeriod:    30 * clocky.Second,
			inventoryRatio: decimal.Zero,
			want:           decimal.Parse("0.5"),
		},
		{
			name:           "full period",
			timeSinceSell:  30 * clocky.Second,
			decayPeriod:    30 * clocky.Second,
			inventoryRatio: decimal.Zero,
			want:           decimal.Zero, // no protection
		},
		{
			name:           "past period",
			timeSinceSell:  60 * clocky.Second,
			decayPeriod:    30 * clocky.Second,
			inventoryRatio: decimal.Zero,
			want:           decimal.Zero, // clamped to zero
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateDecayFactor(DecayLinear, tt.timeSinceSell, tt.decayPeriod, tt.inventoryRatio)
			if got.Cmp(tt.want) != 0 {
				t.Errorf("CalculateDecayFactor(Linear, %v, %v, %v) = %v, want %v",
					tt.timeSinceSell, tt.decayPeriod, tt.inventoryRatio, got, tt.want)
			}
		})
	}
}

func TestCalculateDecayFactor_None(t *testing.T) {
	tests := []struct {
		name          string
		timeSinceSell clocky.Duration
		decayPeriod   clocky.Duration
	}{
		{"zero time", 0, 30 * clocky.Second},
		{"some time", 15 * clocky.Second, 30 * clocky.Second},
		{"past period", 60 * clocky.Second, 30 * clocky.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateDecayFactor(DecayNone, tt.timeSinceSell, tt.decayPeriod, decimal.Zero)
			if got.Cmp(decimal.One) != 0 {
				t.Errorf("CalculateDecayFactor(None, %v, %v, 0) = %v, want 1.0",
					tt.timeSinceSell, tt.decayPeriod, got)
			}
		})
	}
}

func TestCalculateDecayFactor_Exponential(t *testing.T) {
	// Test that exponential mode returns values in [0,1]
	tests := []struct {
		name           string
		timeSinceSell  clocky.Duration
		decayPeriod    clocky.Duration
		inventoryRatio decimal.Decimal
	}{
		{"zero time", 0, 30 * clocky.Second, decimal.Zero},
		{"half period low inv", 15 * clocky.Second, 30 * clocky.Second, decimal.Parse("0.5")},
		{"full period", 30 * clocky.Second, 30 * clocky.Second, decimal.One},
		{"past period", 60 * clocky.Second, 30 * clocky.Second, decimal.Parse("1.5")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateDecayFactor(DecayExponential, tt.timeSinceSell, tt.decayPeriod, tt.inventoryRatio)
			// Should be in [0, 1]
			if got.Cmp(decimal.Zero) < 0 || got.Cmp(decimal.One) > 0 {
				t.Errorf("CalculateDecayFactor(Exponential, %v, %v, %v) = %v, want in [0,1]",
					tt.timeSinceSell, tt.decayPeriod, tt.inventoryRatio, got)
			}
		})
	}
}

func TestBuyGapMetrics_BlockRate(t *testing.T) {
	tests := []struct {
		name     string
		metrics  BuyGapMetrics
		wantRate float64
	}{
		{
			name:     "no signals",
			metrics:  BuyGapMetrics{SignalsTotal: 0, SignalsBlocked: 0, SignalsExecuted: 0},
			wantRate: 0,
		},
		{
			name:     "all blocked",
			metrics:  BuyGapMetrics{SignalsTotal: 100, SignalsBlocked: 100, SignalsExecuted: 0},
			wantRate: 100,
		},
		{
			name:     "none blocked",
			metrics:  BuyGapMetrics{SignalsTotal: 100, SignalsBlocked: 0, SignalsExecuted: 100},
			wantRate: 0,
		},
		{
			name:     "half blocked",
			metrics:  BuyGapMetrics{SignalsTotal: 100, SignalsBlocked: 50, SignalsExecuted: 50},
			wantRate: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.metrics.BlockRate()
			if got != tt.wantRate {
				t.Errorf("BlockRate() = %v, want %v", got, tt.wantRate)
			}
		})
	}
}

func TestBuyGapMetrics_Reset(t *testing.T) {
	m := BuyGapMetrics{
		SignalsTotal:    100,
		SignalsBlocked:  50,
		SignalsExecuted: 50,
	}

	m.Reset()

	if m.SignalsTotal != 0 || m.SignalsBlocked != 0 || m.SignalsExecuted != 0 {
		t.Errorf("Reset() failed: got %+v, want all zeros", m)
	}
}
