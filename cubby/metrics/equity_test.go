package metrics

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"testing"
)

func TestSharpe_Welford_Convergence(t *testing.T) {
	e := NewEquity(clocky.Day)

	// t0: 100
	// t1: 110 (Return 0.1)
	// t2: 132 (Return 0.2)
	// Mean: 0.15
	// Var:  0.005
	startTime := clocky.Date(2025, 1, 1, 0, 0, 0, 0, clocky.TZ)
	inputs := []int64{100, 110, 132}

	for i, val := range inputs {
		ts := startTime.Add(clocky.Day * clocky.Duration(i))
		e.Sample(ts, decimal.FromInt(int(val)))
	}

	sharpe := e.Sharpe(decimal.Zero)

	// Correct Math: 0.15 * sqrt(50400) ~= 33.6749165
	expected := decimal.FromFloat64(33.6749165)

	// Check delta < 0.0001
	delta := sharpe.Sub(expected).Abs()
	epsilon := decimal.FromFloat64(0.0001)

	if delta.Cmp(epsilon) > 0 {
		t.Errorf("Sharpe ratio mismatch.\nGot: %s\nWant: %s", sharpe, expected)
	}
}

func TestSharpe_MinuteQuantum_HighFrequency(t *testing.T) {
	// 1. Setup with Minute quantum
	// This triggers the intraday annualization logic (98,280 periods/year)
	e := NewEquity(clocky.Minute)

	// 2. Data Injection
	// t0: 09:30:00 -> 100
	// t1: 09:31:00 -> 110 (Return 0.1)
	// t2: 09:32:00 -> 132 (Return 0.2)
	startTime := clocky.Date(2025, 1, 1, 9, 30, 0, 0, clocky.TZ)
	inputs := []int64{100, 110, 132}

	for i, val := range inputs {
		// Advance time by 1 Minute per sample
		ts := startTime.Add(clocky.Minute * clocky.Duration(i))
		e.Sample(ts, decimal.FromInt(int(val)))
	}

	// 3. Execution
	sharpe := e.Sharpe(decimal.Zero)

	// 4. Verification
	// Formula: 0.15 * sqrt(98280 / 0.005)
	// Target: 665.0263152...
	expected := decimal.FromFloat64(665.0263152)

	delta := sharpe.Sub(expected).Abs()

	// Allow slightly larger epsilon for the larger magnitude
	epsilon := decimal.FromFloat64(0.001)

	if delta.Cmp(epsilon) > 0 {
		t.Errorf("Sharpe (Minute) mismatch.\nGot: %s\nWant: %s", sharpe, expected)
	}
}
