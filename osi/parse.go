package osi

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
	"errors"
)

var (
	ErrBadOSILength = errors.New("bad osi length")
	ErrBadOSIClass  = errors.New("bad osi class")
	ErrBadOSIStrike = errors.New("bad osi strike")
)

// Parse parses an OSI symbol string into its components. See Encode for the inverse operation.
// e.g. Parse("SPXW  260409C06450000") -> SPXW, 6450, 'C', 2026, 4, 9, nil (schwab and databento style)
// e.g. Parse("SPXW260409C06450000") -> SPXW, 6450, 'C', 2026, 4, 9, nil (alpaca style)
func Parse(osi string) (sym symbol.Symbol, strike decimal.Decimal, class byte, year int, month clocky.Month, day int, err error) {
	n := len(osi)
	if n < 16 || n > 21 {
		return 0, decimal.Zero, 0, 0, 0, 0, ErrBadOSILength
	}
	symEnd := n - 15
	var s uint64
	var shift uint
	for i := 0; i < symEnd && osi[i] != ' '; i++ {
		s |= uint64(osi[i]) << shift
		shift += 8
	}
	sym = symbol.Symbol(s)
	fix := osi[symEnd:]
	var strikeVal int64
	for i := 7; i < 15; i++ {
		c := fix[i]
		if c < '0' || c > '9' {
			return 0, decimal.Zero, 0, 0, 0, 0, ErrBadOSIStrike
		}
		strikeVal = strikeVal*10 + int64(c-'0')
	}
	strike = decimal.FromInt64(strikeVal).DivInt(1000)
	class = fix[6]
	if class != 'C' && class != 'P' {
		return 0, decimal.Zero, 0, 0, 0, 0, ErrBadOSIClass
	}
	year = int(fix[0]-'0')*10 + int(fix[1]-'0') + 2000
	month = clocky.Month(int(fix[2]-'0')*10 + int(fix[3]-'0'))
	day = int(fix[4]-'0')*10 + int(fix[5]-'0')
	return sym, strike, class, year, month, day, nil
}
