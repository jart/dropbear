package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
	"testing"
)

func TestParseOSI(t *testing.T) {
	sym, strike, class, year, month, day, err := parseOSI("SPXW  260409C06450000")
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
