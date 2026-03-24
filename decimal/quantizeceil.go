package decimal

// QuantizeCeil rounds d up (towards +infinity) to a multiple of q.
// Use this for ask prices (never ask less than your limit).
func (d Decimal) QuantizeCeil(q Decimal) Decimal {
	checkQuantum(q)
	qv := int64(q)
	v := int64(d)

	if v <= 0 {
		// for negative, Ceil == Truncate
		return Decimal(v / qv * qv)
	}

	rem := v % qv
	if rem == 0 {
		return d
	}
	return Decimal(v/qv*qv + qv)
}
