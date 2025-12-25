package decimal

// Mul multiplies two decimals.
func (d Decimal) Mul(o Decimal) Decimal {
	// we want (d * o) / Scale, but d * o can overflow int64.
	// split each operand into hi (units) and lo (fraction) parts.
	// then compute the 4 cross-products carefully to avoid overflow.
	dv, ov := int64(d), int64(o)
	dHi, dLo := dv/Scale, dv%Scale
	oHi, oLo := ov/Scale, ov%Scale
	loLo := dLo * oLo / Scale
	result := dHi*oHi*Scale + dHi*oLo + dLo*oHi + loLo
	return Decimal(result)
}
