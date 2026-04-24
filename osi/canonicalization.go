package osi

import "fmt"

// Canonicalize converts an OSI symbol string into a canonical format with fixed spacing.
// This ensures it's the format that's supported by Databento and Schwab.
// If input string is not a valid OSI symbol, it's returned unchanged.
func Canonicalize(osi string) string {
	sym, strike, class, year, month, day, err := Parse(osi)
	if err != nil {
		return osi
	}
	return Encode(sym, strike, class, year, month, day)
}

// Uncanonicalize converts an OSI symbol string into a spaceless format preferred by Alpaca.
// If input string is not a valid OSI symbol, it's returned unchanged.
func Uncanonicalize(osi string) string {
	sym, strike, class, year, month, day, err := Parse(osi)
	if err != nil {
		return osi
	}
	return EncodeSpaceless(sym, strike, class, year, month, day)
}

// Humanize converts an OSI symbol string into a human-friendly format, e.g. "SPXW 2026-04-09 C 645".
func Humanize(osi string) string {
	sym, strike, class, year, month, day, err := Parse(osi)
	if err != nil {
		return osi
	}
	return fmt.Sprintf("%s %04d-%02d-%02d %c %s", sym, year, month, day, class, strike.String())
}
