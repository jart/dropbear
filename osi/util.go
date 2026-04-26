package osi

// IsOptionsSymbol returns true if the given symbol is a valid OSI options symbol.
func IsOptionsSymbol(sym string) bool {
	_, _, _, _, _, _, err := Parse(sym)
	return err == nil
}
