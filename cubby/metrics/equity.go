package metrics

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

// Equity tracks portfolio performance over time for Sharpe ratio calculation.
type Equity struct {
	samples  []decimal.Decimal
	quantum  clocky.Duration
	lastTime clocky.Time
}

// NewEquity creates new equity tracker.
func NewEquity(quantum clocky.Duration) *Equity {
	if quantum < clocky.Millisecond {
		panic("quantum too small")
	}
	return &Equity{quantum: quantum}
}

// ShouldSample returns true if time is right to call Sample.
func (m *Equity) ShouldSample(timestamp clocky.Time) bool {
	return m.lastTime.IsZero() || timestamp.Sub(m.lastTime) >= m.quantum
}

// Sample records value of investment at point in time.
func (m *Equity) Sample(timestamp clocky.Time, value decimal.Decimal) {
	if !value.IsPositive() {
		panic("illegal portfolio value")
	}
	m.samples = append(m.samples, value)
	m.lastTime = timestamp
}

// Sharpe calculates the annualized Sharpe ratio.
func (m *Equity) Sharpe(riskFreeRate decimal.Decimal) decimal.Decimal {
	if len(m.samples) < 3 {
		return decimal.Zero
	}

	// calculate Periods Per Year
	// by enforcing quantum >= 1ms, the max periods per year is ~5.9 billion
	// this fits safely within the 9 billion limit of the decimal library
	var periodsPerYear int64
	if m.quantum >= clocky.Day {
		// Daily sampling: 252 * (Day / Quantum)
		periodsPerYear = 252 * int64(clocky.Day) / int64(m.quantum)
	} else {
		// Intraday: 98,280 trading minutes per year
		// Calculation: 252 days * 6.5 hours * 60 mins * 60 secs * 1e9 nanos
		// = 5,896,800,000,000,000 nanoseconds per trading year
		const tradingNanosPerYear = 98280 * 60 * 1_000_000_000
		periodsPerYear = tradingNanosPerYear / int64(m.quantum)
	}
	if periodsPerYear == 0 {
		return decimal.Zero
	}
	decPeriods := decimal.FromInt(int(periodsPerYear))
	riskFreePerPeriod := riskFreeRate.Div(decPeriods)

	// compute mean and variance using Welford's algorithm (running method)
	// This avoids accumulating a massive sum that could overflow
	//
	// M_k = M_{k-1} + (x_k - M_{k-1}) / k
	// S_k = S_{k-1} + (x_k - M_{k-1}) * (x_k - M_k)
	meanExcess := decimal.Zero
	m2 := decimal.Zero // sum of squares of differences

	// start from i=1 because we need pairs to calculate return
	n := 0

	for i := 1; i < len(m.samples); i++ {
		n++

		// calculate return for this step
		prev := m.samples[i-1]
		curr := m.samples[i]

		// assetReturn = (curr - prev) / prev
		assetReturn := curr.Sub(prev).Div(prev)
		excessReturn := assetReturn.Sub(riskFreePerPeriod)

		// Welford's Update
		// delta = x - mean
		delta := excessReturn.Sub(meanExcess)

		// mean += delta / n
		meanExcess = meanExcess.Add(delta.DivInt(n))

		// delta2 = x - mean (using the NEW mean)
		delta2 := excessReturn.Sub(meanExcess)

		// m2 += delta * delta2
		m2 = m2.Add(delta.Mul(delta2))
	}

	// variance = m2 / (n - 1)
	variance := m2.DivInt(n - 1)
	stdDev := variance.Sqrt()

	// the final calculation
	if stdDev.Cmp(decimal.Epsilon) < 0 {
		return decimal.Zero
	}

	// Sharpe = Mean * Sqrt(Periods) / StdDev
	annualizedFactor := decPeriods.Sqrt()
	return meanExcess.Mul(annualizedFactor).Div(stdDev)
}

// MaxDrawdown calculates the maximum peak-to-trough decline.
func (m *Equity) MaxDrawdown() decimal.Decimal {
	if len(m.samples) < 2 {
		return decimal.Zero
	}
	maxDD := decimal.Zero
	peak := m.samples[0]
	for _, sample := range m.samples {
		if sample.Cmp(peak) > 0 {
			peak = sample
		}
		if peak.IsPositive() {
			// dd = (peak - sample) / peak
			dd := peak.Sub(sample).Div(peak)
			if dd.Cmp(maxDD) > 0 {
				maxDD = dd
			}
		}
	}
	return maxDD
}
