package decimal

import "testing"

func TestPanic(t *testing.T) {

	// Helper to assert that a specific operation panics
	expectPanic := func(name string, op func()) {
		t.Helper()
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("%s: expected panic, but code proceeded successfully", name)
			}
		}()
		op()
	}

	// --- Mul Panics ---

	expectPanic("Mul: Max * 2 (Result Overflow)", func() {
		// Result 2 * MaxInt64 > MaxInt64
		Max.Mul(Decimal(2 * Scale))
	})

	expectPanic("Mul: Min * -1 (Result Overflow)", func() {
		// Result +2^63 (cannot fit in signed int64)
		Min.Mul(NegOne)
	})

	expectPanic("Mul: Max * Max (Intermediate Overflow)", func() {
		// Even with 128-bit math, Max * Max is ~8e37.
		// If Scale is 1e9, result is ~8e28, which fits in 128-bit.
		// But the final quotient ~8e28 definitely exceeds MaxInt64 (~9e18).
		Max.Mul(Max)
	})

	expectPanic("Mul: Overflow by Scale", func() {
		// Max * (1 + epsilon)
		// Multiplier slightly larger than 1 will push Max over the edge
		// 1.000000002
		Multiplier := One + Decimal(2)
		Max.Mul(Multiplier)
	})

	// --- Div Panics ---

	expectPanic("Div: Divide by Zero", func() {
		One.Div(Decimal(0))
	})

	expectPanic("DivEven: Divide by Zero", func() {
		One.DivEven(Decimal(0))
	})

	expectPanic("Div: Max / 0.5 (Result Overflow)", func() {
		// Result Max * 2
		Max.Div(Half)
	})

	expectPanic("Div: Min / -1 (Result Overflow)", func() {
		// Result +2^63
		Min.Div(NegOne)
	})

	expectPanic("Div: Min / 0.5 (Result Overflow)", func() {
		// Result -2^64 (Way too small)
		Min.Div(Half)
	})

	// --- MulInt Panics ---

	expectPanic("MulInt: Max * 2", func() {
		Max.MulInt(2)
	})

	expectPanic("MulInt: Min * -1", func() {
		Min.MulInt(-1)
	})

	// --- DivInt Panics ---

	expectPanic("DivInt: Divide by Zero", func() {
		Max.DivInt(0)
	})

	expectPanic("DivInt: Min / -1", func() {
		Min.DivInt(-1)
	})
}
