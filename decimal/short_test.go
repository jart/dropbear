package decimal

import "testing"

func TestShortDecimal(t *testing.T) {
	tests := []struct {
		short Short
		want  Decimal
	}{
		{0, 0},
		{1, 100},                     // 0.0001 → 0.000100
		{-1, -100},                   // -0.0001 → -0.000100
		{10000, 1_000_000},           // 1.0000 → 1.000000
		{12345, 1_234_500},           // 1.2345 → 1.234500
		{-12345, -1_234_500},         // -1.2345 → -1.234500
		{ShortMax, 214_748_364_700},  // max
		{ShortMin, -214_748_364_800}, // min
	}
	for _, tt := range tests {
		got := tt.short.Decimal()
		if got != tt.want {
			t.Errorf("Short(%d).Decimal() = %d, want %d", tt.short, got, tt.want)
		}
	}
}

func TestDecimalShort(t *testing.T) {
	tests := []struct {
		dec  Decimal
		want Short
	}{
		{0, 0},
		{100, 1},             // 0.000100 → 0.0001
		{-100, -1},           // -0.000100 → -0.0001
		{1_000_000, 10000},   // 1.000000 → 1.0000
		{1_234_500, 12345},   // 1.234500 → 1.2345
		{-1_234_500, -12345}, // -1.234500 → -1.2345
	}
	for _, tt := range tests {
		got := tt.dec.Short()
		if got != tt.want {
			t.Errorf("Decimal(%d).Short() = %d, want %d", tt.dec, got, tt.want)
		}
	}
}

func TestDecimalShortRounding(t *testing.T) {
	// Decimal has 6 places, Short has 4. The bottom 2 digits are rounded
	// using banker's rounding (round half to even).
	tests := []struct {
		name string
		dec  Decimal
		want Short
	}{
		{"round up", 1_234_567, 12346},            // 1.234567 → 1.2346 (67 > 50)
		{"round down", 1_234_523, 12345},          // 1.234523 → 1.2345 (23 < 50)
		{"round up neg", -1_234_567, -12346},      // -1.234567 → -1.2346
		{"round down neg", -1_234_523, -12345},    // -1.234523 → -1.2345
		{"half to even down", 1_234_550, 12346},   // 1.234550 → 1.2346 (5 is odd, round up)
		{"half to even up", 1_234_650, 12346},     // 1.234650 → 1.2346 (6 is even, keep)
		{"half to even neg", -1_234_550, -12346},  // -1.234550 → -1.2346
		{"half to even neg2", -1_234_650, -12346}, // -1.234650 → -1.2346
		{"round up 99", 1_234_599, 12346},         // 1.234599 → 1.2346
		{"round down 01", 1_234_501, 12345},       // 1.234501 → 1.2345
		{"exact", 1_234_500, 12345},               // 1.234500 → 1.2345 (no loss)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dec.Short()
			if got != tt.want {
				t.Errorf("Decimal(%d).Short() = %d, want %d", tt.dec, got, tt.want)
			}
		})
	}
}

func TestDecimalShortRoundtrip(t *testing.T) {
	// Short → Decimal → Short should always be lossless.
	for _, s := range []Short{0, 1, -1, 12345, -12345, ShortMax, ShortMin} {
		got := s.Decimal().Short()
		if got != s {
			t.Errorf("Short(%d) roundtrip = %d", s, got)
		}
	}
}

func TestDecimalShortOverflow(t *testing.T) {
	tests := []struct {
		name string
		dec  Decimal
	}{
		{"positive overflow", Decimal(ShortMax)*100 + 100},
		{"negative overflow", Decimal(ShortMin)*100 - 100},
		{"large positive", 1_000_000_000_000},
		{"large negative", -1_000_000_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Decimal(%d).Short() should panic", tt.dec)
				}
			}()
			tt.dec.Short()
		})
	}
}
