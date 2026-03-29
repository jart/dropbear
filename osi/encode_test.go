package osi

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
	"testing"
)

func TestEncode(t *testing.T) {
	osi := Encode(symbol.MustParse("SPXW"), decimal.FromInt(6450), 'C', 2026, clocky.April, 9)
	if osi != "SPXW  260409C06450000" {
		t.Errorf("osi = %s, want SPXW  260409C06450000", osi)
	}
}

func BenchmarkEncode(b *testing.B) {
	symbol := symbol.MustParse("SPXW")
	strike := decimal.FromInt(6450)
	for i := 0; b.Loop(); i++ {
		_ = Encode(symbol, strike, 'C', 2026, clocky.April, 9)
	}
}
