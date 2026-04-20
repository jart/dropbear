package osi

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
	"testing"
)

func TestParse(t *testing.T) {
	sym, strike, class, year, month, day, err := Parse("SPXW  260409C06450000")
	if err != nil {
		t.Fatal(err)
	}
	if sym != symbol.MustParse("SPXW") {
		t.Errorf("sym = %v, want SPXW", sym)
	}
	if strike.Cmp(decimal.FromInt(6450)) != 0 {
		t.Errorf("strike = %v, want 6450", strike)
	}
	if class != 'C' {
		t.Errorf("class = %c, want C", class)
	}
	if year != 2026 {
		t.Errorf("year = %d, want 2026", year)
	}
	if month != clocky.April {
		t.Errorf("month = %v, want April", month)
	}
	if day != 9 {
		t.Errorf("day = %d, want 9", day)
	}
}

func TestParseAlpaca(t *testing.T) {
	sym, strike, class, year, month, day, err := Parse("FXY260618C00068000")
	if err != nil {
		t.Fatal(err)
	}
	if sym != symbol.MustParse("FXY") {
		t.Errorf("sym = %v, want FXY", sym)
	}
	if strike.Cmp(decimal.FromInt(68)) != 0 {
		t.Errorf("strike = %v, want 68", strike)
	}
	if class != 'C' {
		t.Errorf("class = %c, want C", class)
	}
	if year != 2026 {
		t.Errorf("year = %d, want 2026", year)
	}
	if month != clocky.June {
		t.Errorf("month = %v, want June", month)
	}
	if day != 18 {
		t.Errorf("day = %d, want 18", day)
	}
}

func TestParseErrors(t *testing.T) {
	_, _, _, _, _, _, err := Parse("too short")
	if err == nil {
		t.Error("expected error for too short input")
	}
	_, _, _, _, _, _, err = Parse("SPXW  260409C0645000X") // invalid strike digit
	if err == nil {
		t.Error("expected error for invalid strike digit")
	}
}
