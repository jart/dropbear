package metrics

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
	"math"
)

// Equity tracks portfolio performance over time for Sharpe ratio calculation.
type Equity struct {
	samples  []float64
	quantum  clocky.Duration
	lastTime clocky.Time
}

// NewEquity creates new equity tracker.
func NewEquity(quantum clocky.Duration) *Equity {
	return &Equity{quantum: quantum}
}

// ShouldSample returns true if time is right to call Sample.
func (m *Equity) ShouldSample(timestamp clocky.Time) bool {
	return m.lastTime.IsZero() || timestamp.Sub(m.lastTime) >= m.quantum
}

// Sample records value of investment at point in time.
func (m *Equity) Sample(timestamp clocky.Time, value decimal.Decimal) {
	if !value.IsPositive() {
		loggy.Fatalf("invalid portfolio value for metrics sampling: %s", value)
	}
	// Allow gaps in equity data - equities have market hours
	m.samples = append(m.samples, value.Float64())
	m.lastTime = timestamp
}

// Sharpe calculates the annualized Sharpe ratio.
// Uses ~252 trading days per year for equities.
func (m *Equity) Sharpe(riskFreeRate float64) float64 {
	if len(m.samples) < 3 {
		return 0
	}

	// Calculate periods per year based on sampling frequency
	// For daily or longer sampling: 252 trading days per year
	// For intraday: scale based on 6.5 trading hours per day
	var periodsPerYear float64
	if m.quantum >= clocky.Day {
		// Daily sampling = 252 periods per year
		periodsPerYear = 252.0 * float64(clocky.Day) / float64(m.quantum)
	} else {
		// Intraday: 252 days * 6.5 hours * 60 minutes = 98,280 trading minutes/year
		tradingMinutesPerYear := 252.0 * 6.5 * 60
		periodsPerYear = tradingMinutesPerYear / (float64(m.quantum) / float64(clocky.Minute))
	}
	riskFreeReturn := riskFreeRate / periodsPerYear

	// compute excess returns for each period
	meanExcessReturn := 0.0
	excessReturns := make([]float64, 0, len(m.samples)-1)
	for i := 1; i < len(m.samples); i++ {
		prev := m.samples[i-1]
		curr := m.samples[i]
		assetReturn := (curr - prev) / prev
		excessReturn := assetReturn - riskFreeReturn
		meanExcessReturn += excessReturn
		excessReturns = append(excessReturns, excessReturn)
	}
	meanExcessReturn /= float64(len(excessReturns))

	// compute standard deviation
	variance := 0.0
	for _, r := range excessReturns {
		diff := r - meanExcessReturn
		variance += diff * diff
	}
	variance /= float64(len(excessReturns) - 1)
	stdDevPerPeriod := math.Sqrt(variance)

	if stdDevPerPeriod < 1e-10 {
		return 0
	}

	return meanExcessReturn * math.Sqrt(periodsPerYear) / stdDevPerPeriod
}

// MaxDrawdown calculates the maximum peak-to-trough decline.
func (m *Equity) MaxDrawdown() float64 {
	if len(m.samples) < 2 {
		return 0
	}
	maxDD := 0.0
	peak := m.samples[0]
	for _, sample := range m.samples {
		if sample > peak {
			peak = sample
		}
		if peak > 0 {
			dd := (peak - sample) / peak
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return maxDD
}
