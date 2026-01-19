package decimal

import (
	"math/big"
	"math/rand"
	"testing"
)

const limit = 900_000_000
const iterations = 23_000

func randomCryptoValueImpl(rng *rand.Rand) float64 {
	switch rng.Intn(7) {
	case 0:
		// btc-like prices (e.g., 80,000)
		return 10 + rng.Float64()*1_000_000
	case 1:
		// small fractions (e.g., 0.00000001 to 0.01)
		return rng.Float64() * 0.01
	case 2:
		// really small fractions (e.g., 0.00000001)
		return rng.Float64() * 1e-6
	case 3:
		// medium values (e.g., 0.01 to 1000)
		return 0.01 + rng.Float64()*999.99
	case 4:
		// crypto quantities (e.g., 0.001 to 100)
		return 0.001 + rng.Float64()*99.999
	case 5:
		// larger quantities but still reasonable (e.g., 100 to 10000)
		return 100 + rng.Float64()*9900
	case 6:
		// basis points / fee calculations (e.g., 0.9985, 1.0015)
		return 0.99 + rng.Float64()*0.02
		// NOTE: Values above ~9 trillion require Parse() instead of FromFloat64()
		// due to float64 precision limits. For huge values, use Parse() with strings.
	}
	return rng.Float64()
}

// bigScale is Scale as a big.Int for exact arithmetic
var bigScale = big.NewInt(Scale)

// exactAdd returns a+b if it fits in int64, or (0, false) if it overflows.
func exactAdd(a, b Decimal) (Decimal, bool) {
	result := new(big.Int).Add(big.NewInt(int64(a)), big.NewInt(int64(b)))
	if !result.IsInt64() {
		return 0, false
	}
	return Decimal(result.Int64()), true
}

// exactSub returns a-b if it fits in int64, or (0, false) if it overflows.
func exactSub(a, b Decimal) (Decimal, bool) {
	result := new(big.Int).Sub(big.NewInt(int64(a)), big.NewInt(int64(b)))
	if !result.IsInt64() {
		return 0, false
	}
	return Decimal(result.Int64()), true
}

// exactMul computes a*b using exact arbitrary-precision arithmetic,
// then rounds to our 6 decimal place precision using Banker's Rounding.
// Returns (result, true) if result fits in int64, or (0, false) if it overflows.
func exactMul(a, b Decimal) (Decimal, bool) {
	aBig := big.NewInt(int64(a))
	bBig := big.NewInt(int64(b))
	product := new(big.Int).Mul(aBig, bBig)

	// Divide and get remainder
	quot := new(big.Int)
	rem := new(big.Int)
	quot.QuoRem(product, bigScale, rem)

	// Banker's Rounding (Round Half To Even)
	// If remainder > half, round away from zero.
	// If remainder == half, round to nearest even number.
	remAbs := new(big.Int).Abs(rem)
	halfScale := big.NewInt(Scale / 2)
	cmp := remAbs.Cmp(halfScale)

	if cmp > 0 {
		// |rem| > 0.5: Round away from zero
		if product.Sign() >= 0 {
			quot.Add(quot, big.NewInt(1))
		} else {
			quot.Sub(quot, big.NewInt(1))
		}
	} else if cmp == 0 {
		// |rem| == 0.5: Round to even
		if quot.Bit(0) == 1 { // If odd, round away from zero to make it even
			if product.Sign() >= 0 {
				quot.Add(quot, big.NewInt(1))
			} else {
				quot.Sub(quot, big.NewInt(1))
			}
		}
	}

	if !quot.IsInt64() {
		return 0, false
	}
	return Decimal(quot.Int64()), true
}

// exactDiv computes a/b using exact arbitrary-precision arithmetic with rounding.
func exactDiv(a, b Decimal) Decimal {
	aBig := big.NewInt(int64(a))
	aBig.Mul(aBig, bigScale) // scale up numerator
	bBig := big.NewInt(int64(b))

	// Divide and get remainder for rounding
	quot := new(big.Int)
	rem := new(big.Int)
	quot.QuoRem(aBig, bBig, rem)

	// Round to nearest: if |rem * 2| >= |b|, round away from zero
	rem.Abs(rem)
	rem.Mul(rem, big.NewInt(2))
	bAbs := new(big.Int).Abs(bBig)
	if rem.Cmp(bAbs) >= 0 {
		if (int64(a) >= 0) == (int64(b) >= 0) {
			quot.Add(quot, big.NewInt(1))
		} else {
			quot.Sub(quot, big.NewInt(1))
		}
	}

	return Decimal(quot.Int64())
}

// TestSmoke generates random numbers within cryptocurrency-realistic ranges
// and verifies that our Decimal arithmetic matches exact big.Int arithmetic.
func TestSmoke(t *testing.T) {
	rng := rand.New(rand.NewSource(42)) // deterministic

	for range iterations {
		a := randomCryptoValue(rng)
		b := randomCryptoValue(rng)

		if b == 0 {
			continue
		}

		aDec := FromFloat64(a)
		bDec := FromFloat64(b)

		// Test Mul (skip if result would overflow int64)
		if expectedMul, ok := exactMul(aDec, bDec); ok {
			decMul := aDec.Mul(bDec)
			if decMul != expectedMul {
				t.Errorf("Mul MISMATCH:\n  a = %s (%d)\n  b = %s (%d)\n  expected: %d\n  got: %d",
					aDec.String(), int64(aDec), bDec.String(), int64(bDec),
					int64(expectedMul), int64(decMul))
				return
			}
		}

		// Test Div (skip if b is too small or result would overflow)
		absBDec := int64(bDec)
		if absBDec < 0 {
			absBDec = -absBDec
		}
		if absBDec > 1000 { // avoid division by very small numbers
			expectedDiv := exactDiv(aDec, bDec)
			absExpectedDiv := int64(expectedDiv)
			if absExpectedDiv < 0 {
				absExpectedDiv = -absExpectedDiv
			}
			if absExpectedDiv < limit*Scale {
				decDiv := aDec.Div(bDec)
				if decDiv != expectedDiv {
					t.Errorf("Div MISMATCH:\n  a = %s (%d)\n  b = %s (%d)\n  expected: %d\n  got: %d",
						aDec.String(), int64(aDec), bDec.String(), int64(bDec),
						int64(expectedDiv), int64(decDiv))
					return
				}
			}
		}

		// Test Add (skip if would overflow int64)
		if expectedAdd, ok := exactAdd(aDec, bDec); ok {
			decAdd := aDec.Add(bDec)
			if decAdd != expectedAdd {
				t.Errorf("Add MISMATCH:\n  a = %s\n  b = %s\n  expected: %d\n  got: %d",
					aDec.String(), bDec.String(), int64(expectedAdd), int64(decAdd))
				return
			}
		}

		// Test Sub (skip if would overflow int64)
		if expectedSub, ok := exactSub(aDec, bDec); ok {
			decSub := aDec.Sub(bDec)
			if decSub != expectedSub {
				t.Errorf("Sub MISMATCH:\n  a = %s\n  b = %s\n  expected: %d\n  got: %d",
					aDec.String(), bDec.String(), int64(expectedSub), int64(decSub))
				return
			}
		}
	}
}

// randomCryptoValue generates a random value in cryptocurrency-realistic ranges
func randomCryptoValue(rng *rand.Rand) float64 {
	res := randomCryptoValueImpl(rng)
	if rng.Intn(2) == 0 {
		res = -res
	}
	return res
}

// TestSmokeFloat64Conversion tests that FromFloat64 and Float64 are consistent
func TestSmokeFloat64Conversion(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	const iterations = 10000

	for range iterations {
		// Generate random float in valid range
		f := randomCryptoValue(rng)

		// Convert to Decimal and back
		dec := FromFloat64(f)
		back := dec.Float64()

		// The round-trip should match within float64's precision (~15-16 digits).
		// For large values (billions), we can't preserve all 9 decimal places.
		// Use relative tolerance based on float64's ~2^-52 relative precision.
		diff := f - back
		if diff < 0 {
			diff = -diff
		}
		absF := f
		if absF < 0 {
			absF = -absF
		}

		// Tolerance: max of (1e-6 absolute, 1e-7 relative to magnitude)
		// The 1e-7 relative accounts for float64 having ~16 significant digits
		tolerance := 1e-6
		if rel := absF * 1e-7; rel > tolerance {
			tolerance = rel
		}
		if diff > tolerance {
			t.Errorf("Float64 round-trip failed:\n  input:  %.15f\n  output: %.15f\n  diff:   %.15e\n  dec:    %s",
				f, back, diff, dec.String())
		}

		// Verify String() produces something parseable
		s := dec.String()
		reparsed := Parse(s)
		if reparsed != dec {
			t.Errorf("String/Parse round-trip failed:\n  dec:      %d\n  string:   %s\n  reparsed: %d",
				dec, s, reparsed)
		}
	}
}

// TestSmokeEdgeCases tests specific edge cases that might cause problems
func TestSmokeEdgeCases(t *testing.T) {
	cases := []struct {
		name string
		a, b float64
	}{
		// Cases from actual bugs
		{"fee_calc", 0.001, 0.9985},
		{"small_mul", 0.0001, 0.9985},
		{"near_one", 0.999999999, 0.999999999},
		{"basis_points", 13.16, 0.9997}, // alpaca fee
		{"large_div", 99.82, 13.16},
		{"bps_calc", 10000, 0.0001},

		// BTC edge cases
		{"btc_satoshi", 0.00000001, 100000},
		{"btc_small_qty", 0.00012345, 99500.50}, // small BTC amount * price
		{"btc_price", 100000, 0.12345678},
		{"btc_value_calc", 1.5, 98765.4321}, // 1.5 BTC at price

		// Realistic large values (but within int64 limits for result)
		// Max result is ~9.2 billion due to int64/scale limit
		{"large_price_qty", 100000, 100},   // $100k price * 100 units = $10M
		{"large_notional", 50000, 1000},    // $50k * 1000 = $50M
		{"max_safe_result", 9000, 1000000}, // result = 9B, near limit
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			aDec := FromFloat64(tc.a)
			bDec := FromFloat64(tc.b)

			// Test mul against exact big.Int result
			decMul := aDec.Mul(bDec)
			expectedMul, ok := exactMul(aDec, bDec)
			if !ok {
				t.Fatalf("exactMul overflow for %s", tc.name)
			}
			if decMul != expectedMul {
				t.Errorf("Mul: got %d, expected %d", int64(decMul), int64(expectedMul))
			}

			// Test div against exact big.Int result
			if tc.b != 0 {
				decDiv := aDec.Div(bDec)
				expectedDiv := exactDiv(aDec, bDec)
				if decDiv != expectedDiv {
					t.Errorf("Div: got %d, expected %d", int64(decDiv), int64(expectedDiv))
				}
			}
		})
	}
}
