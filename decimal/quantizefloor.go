package decimal

// QuantizeFloor rounds d down (towards -infinity) to a multiple of q.
// Use this for bid prices (never bid more than your limit).
func (d Decimal) QuantizeFloor(q Decimal) Decimal {
	checkQuantum(q)
	qv := int64(q)
	v := int64(d)

	if v >= 0 {
		// for positive, Floor == Truncate
		return Decimal(v / qv * qv)
	}

	// for negative, we need to push lower if there's a remainder
	rem := v % qv
	if rem == 0 {
		return d
	}

	// go division truncates toward zero (e.g. -12/5 = -2).
	// we want -3 * 5 = -15.
	return Decimal(v/qv*qv - qv)
}
