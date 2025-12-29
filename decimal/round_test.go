package decimal

import (
	"math"
	"testing"
)

func TestDecimal_RoundNearest(t *testing.T) {
	tests := []struct {
		name      string
		d         Decimal
		want      Decimal
		wantPanic bool
	}{
		{"simple up", Parse("1.6"), Parse("2"), false},
		{"simple down", Parse("1.4"), Parse("1"), false},
		{"negative up", Parse("-1.6"), Parse("-2"), false},
		{"negative down", Parse("-1.4"), Parse("-1"), false},
		{"half up", Parse("2.5"), Parse("3"), false},
		{"half down", Parse("2.4"), Parse("2"), false},
		{"negative half up", Parse("-2.5"), Parse("-3"), false},
		{"negative half down", Parse("-2.4"), Parse("-2"), false},
		{"negative six #1", Parse("-5.5"), Parse("-6"), false},
		{"negative six #2", Parse("-5.6"), Parse("-6"), false},
		{"zero", Parse("0.0"), Parse("0"), false},
		{"large number", Parse("123456789.5"), Parse("123456790"), false},
		{"large negative number", Parse("-123456789.5"), Parse("-123456790"), false},
		{"maximum value", Max, 0, true},
		{"minimum value", Min, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Decimal.RoundNearest() did not panic")
					}
				}()
			}
			if got := tt.d.RoundNearest(); got != tt.want {
				t.Errorf("Decimal.RoundNearest(%v) = %v (%d), want %v (%d)", tt.d, got, int64(got), tt.want, int64(tt.want))
			}
		})
	}
}

func TestDecimal_QuantizeNearest_MinIntOverflow(t *testing.T) {
	t.Run("Dead Code Verification", func(t *testing.T) {
		// This confirms that passing -1 as quantum with MinInt64
		// panics at the DIVISION stage, long before the multiplication check.
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected division panic, got none")
			}
		}()
		d := Decimal(math.MinInt64)
		q := Decimal(-1)
		// This will panic with "runtime error: integer divide overflow" on most CPUs
		d.QuantizeNearest(q)
	})
}

func TestDecimal_QuantizeNearest_Overflows(t *testing.T) {
	tests := []struct {
		name      string
		d, q      Decimal
		wantPanic bool
	}{
		{
			// MaxInt64 is 9,223,372,036,854,775,807.
			// Scale (10^9) is 1,000,000,000.
			// Max / Scale = 9,223,372,036.
			// Remainder = 854,775,807.
			// Threshold = 500,000,000.
			// 854M > 500M -> Round UP.
			// Result becomes 9,223,372,037 * 10^9 = 9,223,372,037,000,000,000.
			// This is > MaxInt64. MUST PANIC.
			name:      "MaxInt64 round up overflow (Scale)",
			d:         Decimal(math.MaxInt64),
			q:         Decimal(Scale),
			wantPanic: true,
		},
		{
			// MinInt64 is -9,223,372,036,854,775,808.
			// Remainder logic mirrors the positive side.
			// It will try to round to -9,223,372,037 * 10^9.
			// This is < MinInt64. MUST PANIC.
			name:      "MinInt64 round down overflow (Scale)",
			d:         Decimal(math.MinInt64),
			q:         Decimal(Scale),
			wantPanic: true,
		},
		{
			// TIGHT BOUNDARY TEST
			// MaxInt64 ends in ...807.
			// q = 10. Half = 5.
			// Remainder = 7.
			// 7 >= 5 -> Round UP.
			// Current: ...800 + 7
			// Next multiple of 10: ...810.
			// ...810 > ...807 (MaxInt64). Overflow.
			name:      "MaxInt64 round up overflow (Small Quantum)",
			d:         Decimal(math.MaxInt64),
			q:         Decimal(10),
			wantPanic: true,
		},
		{
			// SAFE CASE (Regression Test)
			// MaxInt64 ends in ...807.
			// q = 100. Half = 50.
			// Remainder = 7.
			// 7 < 50 -> Round DOWN (Truncate).
			// Result: ...800.
			// ...800 < ...807. Safe.
			name:      "MaxInt64 round down safe (Small Quantum)",
			d:         Decimal(math.MaxInt64),
			q:         Decimal(100),
			wantPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("QuantizeNearest() panic = %v, wantPanic %v", r, tt.wantPanic)
				}
			}()

			// If it doesn't panic, we assume the result is valid (checked by other tests)
			// We are only testing the crash/no-crash boundary here.
			tt.d.QuantizeNearest(tt.q)
		})
	}
}

func TestDecimal_RoundEven(t *testing.T) {
	tests := []struct {
		name      string
		d         Decimal
		want      Decimal
		wantPanic bool
	}{
		{"simple up", Parse("1.6"), Parse("2"), false},
		{"simple down", Parse("1.4"), Parse("1"), false},
		{"negative woo", Parse("-1.5"), Parse("-2"), false},
		{"negative up", Parse("-1.6"), Parse("-2"), false},
		{"negative down", Parse("-1.4"), Parse("-1"), false},
		{"half even", Parse("2.5"), Parse("2"), false},
		{"half down", Parse("2.4"), Parse("2"), false},
		{"negative half even", Parse("-2.5"), Parse("-2"), false},
		{"negative half down", Parse("-2.4"), Parse("-2"), false},
		{"negative six #1", Parse("-5.5"), Parse("-6"), false},
		{"negative six #2", Parse("-5.6"), Parse("-6"), false},
		{"zero", Parse("0.0"), Parse("0"), false},
		{"large number", Parse("123456789.5"), Parse("123456790"), false},
		{"large negative number", Parse("-123456789.5"), Parse("-123456790"), false},
		{"maximum value", Max, 0, true},
		{"minimum value", Min, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Decimal.RoundEven() did not panic")
					}
				}()
			}
			if got := tt.d.RoundEven(); got != tt.want {
				t.Errorf("Decimal.RoundEven(%v) = %v (%d), want %v (%d)", tt.d, got, int64(got), tt.want, int64(tt.want))
			}
		})
	}
}

func TestDecimal_Floor(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		want Decimal
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Floor(); got != tt.want {
				t.Errorf("Decimal.Floor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecimal_Ceil(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		want Decimal
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Ceil(); got != tt.want {
				t.Errorf("Decimal.Ceil() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecimal_Truncate(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		want Decimal
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Truncate(); got != tt.want {
				t.Errorf("Decimal.Truncate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecimal_RoundToNearest(t *testing.T) {
	type args struct {
		precision int
	}
	tests := []struct {
		name string
		d    Decimal
		args args
		want Decimal
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.RoundToNearest(tt.args.precision); got != tt.want {
				t.Errorf("Decimal.RoundToNearest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecimal_RoundToEven(t *testing.T) {
	type args struct {
		precision int
	}
	tests := []struct {
		name string
		d    Decimal
		args args
		want Decimal
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.RoundToEven(tt.args.precision); got != tt.want {
				t.Errorf("Decimal.RoundToEven() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecimal_FloorTo(t *testing.T) {
	type args struct {
		precision int
	}
	tests := []struct {
		name string
		d    Decimal
		args args
		want Decimal
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.FloorTo(tt.args.precision); got != tt.want {
				t.Errorf("Decimal.FloorTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecimal_CeilTo(t *testing.T) {
	type args struct {
		precision int
	}
	tests := []struct {
		name string
		d    Decimal
		args args
		want Decimal
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.CeilTo(tt.args.precision); got != tt.want {
				t.Errorf("Decimal.CeilTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecimal_TruncateTo(t *testing.T) {
	type args struct {
		precision int
	}
	tests := []struct {
		name string
		d    Decimal
		args args
		want Decimal
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.TruncateTo(tt.args.precision); got != tt.want {
				t.Errorf("Decimal.TruncateTo() = %v, want %v", got, tt.want)
			}
		})
	}
}
