package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

// DecayMode specifies how buygap protection decays over time after sells
type DecayMode int

const (
	DecayExponential DecayMode = iota // e^(-timeRatio*periodScale) - default
	DecayLinear                       // max(0, 1-timeRatio) - simpler for short timescales
	DecayNone                         // 1.0 always - no decay (constant gap)
)

// String returns the name of the decay mode
func (d DecayMode) String() string {
	switch d {
	case DecayExponential:
		return "exponential"
	case DecayLinear:
		return "linear"
	case DecayNone:
		return "none"
	default:
		return "unknown"
	}
}

// ParseDecayMode parses a string into a DecayMode
func ParseDecayMode(s string) DecayMode {
	switch s {
	case "exponential":
		return DecayExponential
	case "linear":
		return DecayLinear
	case "none":
		return DecayNone
	default:
		return DecayExponential // default to current behavior
	}
}

// CalculateDecayFactor computes the decay factor for buygap protection
// Returns a value in [0, 1] where:
//   - 1.0 = full gap protection (just sold, don't buy near same price)
//   - 0.0 = no gap protection (enough time has passed)
//
// Parameters:
//   - mode: decay algorithm to use
//   - timeSinceSell: duration since last sell
//   - decayPeriod: base decay period (e.g., 30s)
//   - inventoryRatio: current inventory / target (used for exponential mode)
func CalculateDecayFactor(mode DecayMode, timeSinceSell clocky.Duration, decayPeriod clocky.Duration, inventoryRatio decimal.Decimal) decimal.Decimal {
	if timeSinceSell <= 0 || decayPeriod <= 0 {
		return decimal.One
	}

	switch mode {
	case DecayExponential:
		// Current behavior: e^(-timeRatio*periodScale)
		// periodScale = e^(-inventoryRatio*2)
		// This makes decay slower when inventory is high
		timeRatio := decimal.FromInt(int(timeSinceSell)).Div(decimal.FromInt(int(decayPeriod)))
		periodScale := inventoryRatio.MulInt(2).Neg().Exp()   // e^(-invRatio*2)
		decayFactor := timeRatio.Mul(periodScale).Neg().Exp() // e^(-timeRatio*periodScale)
		return decayFactor.Max(decimal.Zero).Min(decimal.One)

	case DecayLinear:
		// Linear decay: 1 - (timeSinceSell / decayPeriod)
		// Simpler, more predictable for short timescales
		timeRatio := decimal.FromInt(int(timeSinceSell)).Div(decimal.FromInt(int(decayPeriod)))
		decayFactor := decimal.One.Sub(timeRatio)
		return decayFactor.Max(decimal.Zero).Min(decimal.One)

	case DecayNone:
		// No decay - always full gap protection
		return decimal.One

	default:
		return decimal.One
	}
}

// BuyGapMetrics tracks gap protection statistics
type BuyGapMetrics struct {
	SignalsTotal    int // Total buy signals evaluated
	SignalsBlocked  int // Blocked by gap protection
	SignalsExecuted int // Passed gap check
}

// BlockRate returns the percentage of signals blocked
func (m *BuyGapMetrics) BlockRate() float64 {
	if m.SignalsTotal == 0 {
		return 0
	}
	return float64(m.SignalsBlocked) / float64(m.SignalsTotal) * 100
}

// Reset clears the metrics
func (m *BuyGapMetrics) Reset() {
	m.SignalsTotal = 0
	m.SignalsBlocked = 0
	m.SignalsExecuted = 0
}
