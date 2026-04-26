package osi

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
