package decimal

// Half divides by two with banker's rounding.
// This function is equivalent to DivInt(2) and never panics.
func (d Decimal) Half() Decimal {
	x := int64(d)
	h := x / 2
	h += x & h & 1 * (1 | x>>63)
	return Decimal(h)
}
