package decimal

// QuantizeTruncate rounds d toward zero to a multiple of q.
// This is consistent with IEEE 754 "round toward zero".
// Also known as rounding down in Python.
// Use this for order sizing.
func (d Decimal) QuantizeTruncate(q Decimal) Decimal {
	checkQuantum(q)
	return Decimal(int64(d) / int64(q) * int64(q))
}

func checkQuantum(q Decimal) {
	if q <= 0 {
		panic("illegal quantum")
	}
}
