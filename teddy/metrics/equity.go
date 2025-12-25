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

// NewMetrics creates new metrics tracker.
func NewEquity(quantum clocky.Duration) *Equity {
	return &Equity{quantum: quantum}
}

// ShouldSample returns true if time is right to call Sample.
func (m *Equity) ShouldSample(timestamp clocky.Time) bool {
	return m.lastTime.IsZero() || timestamp.Sub(m.lastTime) >= m.quantum
}

// Sample records value of investment at point in time.
// Call ShouldSample first to ensure evenly spaced samples.
// Data must be generated at least as fast as -quantum flag says.
func (m *Equity) Sample(timestamp clocky.Time, value decimal.Decimal) {
	// equity must always be positive, otherwise sharpe calculation is invalid
	if !value.IsPositive() {
		loggy.Fatalf("invalid portfolio value for metrics sampling: %s", value)
	}
	// check for gaps - if we skipped more than 2x the interval, something is wrong
	if !m.lastTime.IsZero() {
		gap := timestamp.Sub(m.lastTime)
		if gap > m.quantum*2 {
			loggy.Fatalf("collective gap in backtest data detected: %s since last sample (quantum=%s)", gap, m.quantum)
		}
		if gap < m.quantum {
			loggy.Fatalf("sample api is being called too frequently: %s since last sample (quantum=%s)", gap, m.quantum)
		}
	}
	m.samples = append(m.samples, value.Float64())
	m.lastTime = timestamp
}

// Sharpe calculates the annualized Sharpe ratio from sampled deep fucking value.
// Uses risk-free rate from your -rfr flag. Sample interval controlled by -quantum.
// This function will panic if the backtesting datasets yielded insufficient samples.
func (m *Equity) Sharpe(riskFreeRate float64) float64 {
	if len(m.samples) < 3 {
		loggy.Fatalf("insufficient samples (%d) for sharpe calculation", len(m.samples))
	}

	// TODO(jart): 360d should be however much trading time there is in a year.
	//             for crypto this is pretty much correct but fer equities that
	//             would be 1638h if we only want to trade during daytime hours
	microsecondsPerYear := float64(360 * clocky.Day)
	periodsPerYear := microsecondsPerYear / float64(m.quantum)
	riskFreeReturn := riskFreeRate / periodsPerYear

	// compute excess returns (return - risk-free rate) for each period
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

	// compute standard deviation of excess returns
	variance := 0.0
	for _, r := range excessReturns {
		diff := r - meanExcessReturn
		variance += diff * diff
	}
	variance /= float64(len(excessReturns) - 1)
	stdDevPerPeriod := math.Sqrt(variance)

	// handle zero volatility edge case
	if stdDevPerPeriod < 1e-10 {
		if meanExcessReturn > 0 {
			return +100
		} else if meanExcessReturn < 0 {
			return -100
		}
		return 0
	}

	// annualize our sharpie ratio
	return meanExcessReturn * math.Sqrt(periodsPerYear) / stdDevPerPeriod
}

// MaxDrawdown calculates the maximum peak-to-trough decline.
// Returns as a decimal (e.g., 0.15 = 15% drawdown).
func (m *Equity) MaxDrawdown() float64 {
	if len(m.samples) < 2 {
		return 0
	}
	maxDD := 0.0
	peak := m.samples[0]
	for _, sample := range m.samples {
		value := sample
		if value > peak {
			peak = value
		}
		if peak > 0 {
			dd := (peak - value) / peak
			if dd > maxDD {
				maxDD = dd
			}
		}
	}
	return maxDD
}
