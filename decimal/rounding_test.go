package decimal

import (
	"testing"
)

const (
	// Number of satoshi values to test (1 satoshi to N satoshis)
	maxSatoshis = 1_000_000

	// Bitcoin has 8 decimal places (1 satoshi = 0.00000001 BTC)
	satoshiDivisor = 100_000_000
)

// TestfromFloat64Precision tests that fromFloat64 correctly converts
// satoshi-precision values without losing the least significant digit.
func TestFromFloat64Precision(t *testing.T) {
	var errors int
	var firstErrors []int64

	for i := int64(1); i <= maxSatoshis; i++ {
		// Simulate: parse "0.00000017" as float64, convert to Decimal
		f := float64(i) / satoshiDivisor
		d := fromFloat64(f)

		// Expected: i satoshis * (scale / satoshiDivisor) = i * 100
		expected := Decimal(i * (Scale / satoshiDivisor))

		if d != expected {
			if len(firstErrors) < 10 {
				firstErrors = append(firstErrors, i)
			}
			errors++
		}
	}

	if errors > 0 {
		t.Errorf("fromFloat64 precision errors: %d out of %d values (%.2f%%)",
			errors, maxSatoshis, float64(errors)/float64(maxSatoshis)*100)
		t.Errorf("First failing satoshi values: %v", firstErrors)

		// Show details for first error
		i := firstErrors[0]
		f := float64(i) / satoshiDivisor
		d := fromFloat64(f)
		expected := Decimal(i * (Scale / satoshiDivisor))
		t.Errorf("Example: %d satoshis = %.8f BTC", i, f)
		t.Errorf("  fromFloat64 returned: %d", d)
		t.Errorf("  Expected:             %d", expected)
		t.Errorf("  Difference:           %d", d-expected)
	}
}

// TestfromFloat64RoundTrip tests that values survive float64 -> Decimal -> float64.
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
		d := fromFloat64(f)
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
