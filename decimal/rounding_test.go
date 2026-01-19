package decimal

import (
	"testing"
)

const (
	// Number of micro values to test
	maxMicros = 1_000_000

	// Scale is 6 decimal places (1 micro = 0.000001)
	microDivisor = 1_000_000
)

// TestFromFloat64Precision tests that FromFloat64 correctly converts
// micro-precision values without losing the least significant digit.
func TestFromFloat64Precision(t *testing.T) {
	var errors int
	var firstErrors []int64

	for i := int64(1); i <= maxMicros; i++ {
		// Simulate: parse "0.000001" as float64, convert to Decimal
		f := float64(i) / microDivisor
		d := FromFloat64(f)

		// Expected: i micros * (scale / microDivisor) = i * 1 = i
		expected := Decimal(i)

		if d != expected {
			if len(firstErrors) < 10 {
				firstErrors = append(firstErrors, i)
			}
			errors++
		}
	}

	if errors > 0 {
		t.Errorf("FromFloat64 precision errors: %d out of %d values (%.2f%%)",
			errors, maxMicros, float64(errors)/float64(maxMicros)*100)
		t.Errorf("First failing micro values: %v", firstErrors)

		// Show details for first error
		i := firstErrors[0]
		f := float64(i) / microDivisor
		d := FromFloat64(f)
		expected := Decimal(i)
		t.Errorf("Example: %d micros = %.6f", i, f)
		t.Errorf("  FromFloat64 returned: %d", d)
		t.Errorf("  Expected:             %d", expected)
		t.Errorf("  Difference:           %d", d-expected)
	}
}

// TestFromFloat64RoundTrip tests that values survive float64 -> Decimal -> float64.
func TestFromFloat64RoundTrip(t *testing.T) {
	testCases := []float64{
		0.00000001, // 1 satoshi
		0.00000017, // 17 satoshis (known problematic)
		0.00000029, // 29 satoshis (known problematic)
		0.12345678, // 8 decimal places
		1.23456789, // 9 decimal places (Alpaca)
		99999.99999999,
	}

	for _, f := range testCases {
		d := FromFloat64(f)
		back := d.Float64()

		// Allow for precision loss beyond 9 decimal places
		diff := back - f
		if diff < 0 {
			diff = -diff
		}

		// Tolerance: 1 unit in the last place of our 9-decimal representation
		tolerance := 1.0 / Scale
		if diff > tolerance {
			t.Errorf("Round-trip failed for %.10f: got %.10f (diff: %e)",
				f, back, back-f)
		}
	}
}
