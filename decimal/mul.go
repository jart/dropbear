package decimal

// Mul multiplies two decimals, panicking on overflow.
func (d Decimal) Mul(o Decimal) Decimal {
	// we want (d * o) / Scale, but d * o can overflow int64.
	// split each operand into hi (units) and lo (fraction) parts.
	// then compute the 4 cross-products carefully to avoid overflow.
	dv, ov := int64(d), int64(o)
	dHi, dLo := dv/Scale, dv%Scale
	oHi, oLo := ov/Scale, ov%Scale

	// check if dHi*oHi would overflow when multiplied by Scale
	hiHi := dHi * oHi
	if hiHi != 0 && ((hiHi < 0) != ((dHi < 0) != (oHi < 0))) {
		panic("decimal overflow")
	}
	hiHiScaled := hiHi * Scale
	if hiHi != 0 && hiHiScaled/hiHi != Scale {
		panic("decimal overflow")
	}

	// compute cross terms (these can't overflow since |dLo|, |oLo| < Scale)
	hiLo := dHi * oLo
	loHi := dLo * oHi
	loLo := dLo * oLo / Scale

	// sum with overflow checking
	result := hiHiScaled
	for _, term := range []int64{hiLo, loHi, loLo} {
		z := result + term
		if ((z ^ result) & (z ^ term)) < 0 {
			panic("decimal overflow")
		}
		result = z
	}

	return Decimal(result)
}
