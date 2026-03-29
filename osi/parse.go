package osi

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
	"errors"
)

// Parse parses an OSI symbol string into its components. See Encode for the inverse operation.
// e.g. Parse("SPXW  260409C06450000") -> SPXW, 6450, 'C', 2026, 4, 9, nil
func Parse(osi string) (sym symbol.Symbol, strike decimal.Decimal, class byte, year int, month clocky.Month, day int, err error) {
	if len(osi) != 21 {
		return 0, decimal.Zero, 0, 0, 0, 0, errors.New("bad osi length")
	}
	var s uint64
	var shift uint
	for i := 0; i < 6 && osi[i] != ' '; i++ {
		s |= uint64(osi[i]) << shift
		shift += 8
	}
	sym = symbol.Symbol(s)
	var strikeVal int64
	for i := 13; i < 21; i++ {
		c := osi[i]
		if c < '0' || c > '9' {
			return 0, decimal.Zero, 0, 0, 0, 0, errors.New("bad osi strike digit")
		}
		strikeVal = strikeVal*10 + int64(c-'0')
	}
	strike = decimal.FromInt64(strikeVal).DivInt(1000)
	class = osi[12]
	year = int(osi[6]-'0')*10 + int(osi[7]-'0') + 2000
	month = clocky.Month(int(osi[8]-'0')*10 + int(osi[9]-'0'))
	day = int(osi[10]-'0')*10 + int(osi[11]-'0')
	return sym, strike, class, year, month, day, nil
}
