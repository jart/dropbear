package decimal

import (
	"testing"
)

func TestDivEven(t *testing.T) {
	// Scale = 1_000_000_000 (1e9)
	// One  = 1.0 (int64 1,000,000,000)
	// Unit = 1e-9 (int64 1)

	unit := Decimal(1)
	two := Decimal(2 * Scale) // 2.0

	tests := []struct {
		name     string
		d, o     Decimal
		wantDiv  Decimal // Standard (Away from zero)
		wantEven Decimal // Banker's (To Even)
	}{
		// --- THE MICRO GAUNTLET: Rounding at the 1e-9 limit ---

		{
			// Math: 1e-9 / 2.0 = 0.5e-9 (Half a nanocent)
			// Integer Result: 0
			// Remainder: Exactly half
			name:     "0.5 nanocents -> Div:1, Even:0",
			d:        unit,       // 1
			o:        two,        // 2,000,000,000
			wantDiv:  Decimal(1), // Rounds Away to 1 nanocent
			wantEven: Decimal(0), // Rounds to Nearest Even (0)
		},
		{
			// Math: 3e-9 / 2.0 = 1.5e-9 (1.5 nanocents)
			// Integer Result: 1 (Odd)
			// Remainder: Exactly half
			name:     "1.5 nanocents -> Div:2, Even:2",
			d:        Decimal(3),
			o:        two,
			wantDiv:  Decimal(2), // Rounds Away to 2
			wantEven: Decimal(2), // Rounds to Nearest Even (2)
		},
		{
			// Math: 5e-9 / 2.0 = 2.5e-9 (2.5 nanocents)
			// Integer Result: 2 (Even)
			// Remainder: Exactly half
			name:     "2.5 nanocents -> Div:3, Even:2",
			d:        Decimal(5),
			o:        two,
			wantDiv:  Decimal(3), // Rounds Away to 3
			wantEven: Decimal(2), // Rounds to Nearest Even (2)
		},

		// --- Negative Micro Cases ---
		{
			// Math: -1e-9 / 2.0 = -0.5e-9
			name:     "-0.5 nanocents -> Div:-1, Even:0",
			d:        -unit,
			o:        two,
			wantDiv:  Decimal(-1),
			wantEven: Decimal(0),
		},
		{
			// Math: -5e-9 / 2.0 = -2.5e-9
			name:     "-2.5 nanocents -> Div:-3, Even:-2",
			d:        Decimal(-5),
			o:        two,
			wantDiv:  Decimal(-3),
			wantEven: Decimal(-2),
		},

		// --- Real World: 1/3 splits ---
		{
			// 1.0 / 3.0 = 0.333333333333...
			// Library Scale is 1e9.
			// Result should be 0.333333333 (exact integer 333,333,333)
			// The next digit is 3 (less than 5), so both round down.
			name:     "1.0 / 3.0",
			d:        One,
			o:        Decimal(3 * Scale),
			wantDiv:  Decimal(333333333),
			wantEven: Decimal(333333333),
		},
		{
			// 2.0 / 3.0 = 0.666666666666...
			// Result should be 0.666666666...
			// The remainder is 2/3 of divisor ( > 0.5 ).
			// Both methods should round UP to ...667
			name:     "2.0 / 3.0",
			d:        Decimal(2 * Scale),
			o:        Decimal(3 * Scale),
			wantDiv:  Decimal(666666667),
			wantEven: Decimal(666666667),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check Div
			if got := tt.d.Div(tt.o); got != tt.wantDiv {
				t.Errorf("Div() = %v, want %v", got, tt.wantDiv)
			}
			// Check DivEven
			if got := tt.d.DivEven(tt.o); got != tt.wantEven {
				t.Errorf("DivEven() = %v, want %v", got, tt.wantEven)
			}
		})
	}
}
