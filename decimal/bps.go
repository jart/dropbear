package decimal

// FromBPS converts basis points to a Decimal fraction. FromBPS(100) returns 0.01.
func FromBPS(n int) Decimal {
	return Decimal(int64(n) * (Scale / 10000))
}

// ParseBPS parses a string representing basis points into a Decimal fraction.
// For example, "6.5" becomes 0.00065.
func ParseBPS(s string) Decimal {
	return Parse(s).DivInt(10000)
}

// BPS multiplies d by 10,000, for displaying to humans in basis points (1/100 of a percent).
func (d Decimal) BPS() Decimal {
	return d.MulInt(10000)
}
