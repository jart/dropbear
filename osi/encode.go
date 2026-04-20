package osi

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
)

// Encode encodes the given option parameters into an OSI symbol string.
// e.g. Encode(SPY, 400.50, 'C', 2026, 4, 17) -> "SPY   260417C000400500"
// This takes about 23 nanoseconds per call on an Apple M2 microprocessor.
func Encode(sym symbol.Symbol, strike decimal.Decimal, class byte, year int, month clocky.Month, day int) string {
	var buf [21]byte
	// symbol: 6 chars, space-padded
	for i := range buf[:6] {
		buf[i] = ' '
	}
	s := uint64(sym)
	for i := 0; i < 6 && s != 0; i++ {
		buf[i] = byte(s)
		s >>= 8
	}
	// expiration: YYMMDD
	yy := year - 2000
	buf[6] = byte(yy/10) + '0'
	buf[7] = byte(yy%10) + '0'
	buf[8] = byte(month/10) + '0'
	buf[9] = byte(month%10) + '0'
	buf[10] = byte(day/10) + '0'
	buf[11] = byte(day%10) + '0'
	buf[12] = class
	// strike × 1000, 8 digits zero-padded
	x := strike.MulInt(1000).Int64()
	for i := 20; i >= 13; i-- {
		buf[i] = byte(x%10) + '0'
		x /= 10
	}
	return string(buf[:])
}

// EncodeSpaceless encodes the given option parameters into an OSI symbol string.
// e.g. EncodeSpaceless(SPY, 400.50, 'C', 2026, 4, 17) -> "SPY260417C000400500"
func EncodeSpaceless(sym symbol.Symbol, strike decimal.Decimal, class byte, year int, month clocky.Month, day int) string {
	var buf [21]byte
	i := 0
	s := uint64(sym)
	for i < 6 && s != 0 {
		buf[i] = byte(s)
		s >>= 8
		i++
	}
	// expiration: YYMMDD
	yy := year - 2000
	buf[i+0] = byte(yy/10) + '0'
	buf[i+1] = byte(yy%10) + '0'
	buf[i+2] = byte(month/10) + '0'
	buf[i+3] = byte(month%10) + '0'
	buf[i+4] = byte(day/10) + '0'
	buf[i+5] = byte(day%10) + '0'
	buf[i+6] = class
	// strike × 1000, 8 digits zero-padded
	x := strike.MulInt(1000).Int64()
	for j := i + 14; j >= i+7; j-- {
		buf[j] = byte(x%10) + '0'
		x /= 10
	}
	return string(buf[:i+15])
}
