package decimal

import "testing"

func TestShortDecimal(t *testing.T) {
	tests := []struct {
		short Short
		want  Decimal
	}{
		{0, 0},
		{1, Decimal(shortRatio)},
		{-1, -Decimal(shortRatio)},
		{10000, 10000 * Decimal(shortRatio)},
		{12345, 12345 * Decimal(shortRatio)},
		{-12345, -12345 * Decimal(shortRatio)},
		{ShortMax, Decimal(ShortMax) * shortRatio},
		{ShortMin, Decimal(ShortMin) * shortRatio},
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
		{Decimal(shortRatio), 1},
		{-Decimal(shortRatio), -1},
		{10000 * Decimal(shortRatio), 10000},
		{12345 * Decimal(shortRatio), 12345},
		{-12345 * Decimal(shortRatio), -12345},
	}
	for _, tt := range tests {
		got := tt.dec.Short()
		if got != tt.want {
			t.Errorf("Decimal(%d).Short() = %d, want %d", tt.dec, got, tt.want)
		}
	}
}

func TestDecimalShortRounding(t *testing.T) {
	tests := []struct {
		name string
		dec  Decimal
		want Short
	}{
		{"round up", Decimal(1234)*shortRatio + (shortRatio * 3 / 4), 1235},
		{"round down", Decimal(1234)*shortRatio + (shortRatio * 1 / 4), 1234},
		{"round up neg", -(Decimal(1234)*shortRatio + (shortRatio * 3 / 4)), -1235},
		{"round down neg", -(Decimal(1234)*shortRatio + (shortRatio * 1 / 4)), -1234},
		{"half to even (even)", Decimal(1234)*shortRatio + shortRatio/2, 1234},
		{"half to even (odd)", Decimal(1235)*shortRatio + shortRatio/2, 1236},
		{"half to even neg (even)", -(Decimal(1234)*shortRatio + shortRatio/2), -1234},
		{"half to even neg (odd)", -(Decimal(1235)*shortRatio + shortRatio/2), -1236},
		{"round up max", Decimal(1234)*shortRatio + shortRatio - 1, 1235},
		{"round down min", Decimal(1234)*shortRatio + 1, 1234},
		{"exact", Decimal(12345) * shortRatio, 12345},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dec.Short()
			if got != tt.want {
				t.Errorf("Decimal(%d).Short() = %d, want %d (ratio %d)", tt.dec, got, tt.want, shortRatio)
			}
		})
	}
}

func TestDecimalShortRoundtrip(t *testing.T) {
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
		{"positive overflow", Decimal(ShortMax)*shortRatio + shortRatio},
		{"negative overflow", Decimal(ShortMin)*shortRatio - shortRatio},
		{"large positive", Max},
		{"large negative", Min},
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

func TestDecimalCanShort(t *testing.T) {
	tests := []struct {
		dec  Decimal
		want bool
	}{
		{0, true},
		{Decimal(shortRatio), true},
		{-Decimal(shortRatio), true},
		{10000 * Decimal(shortRatio), true},
		{12345 * Decimal(shortRatio), true},
		{-12345 * Decimal(shortRatio), true},
		{Decimal(ShortMax) * shortRatio, true},
		{Decimal(ShortMin) * shortRatio, true},
		{Decimal(ShortMax)*shortRatio + shortRatio, false},
		{Decimal(ShortMin)*shortRatio - shortRatio, false},
	}
	for _, tt := range tests {
		got := tt.dec.CanShort()
		if got != tt.want {
			t.Errorf("Decimal(%d).CanShort() = %t, want %t", tt.dec, got, tt.want)
		}
	}
}

func TestShortString(t *testing.T) {
	tests := []struct {
		short Short
		want  string
	}{
		{0, "0"},
		{1 * ShortScale, "1"},
		{123 * ShortScale / 100, "1.23"},
		{-123 * ShortScale / 100, "-1.23"},
	}
	for _, tt := range tests {
		got := tt.short.String()
		if got != tt.want {
			t.Errorf("Short(%d).String() = %s, want %s", tt.short, got, tt.want)
		}
	}
}
