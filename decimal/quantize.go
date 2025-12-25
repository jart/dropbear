package decimal

// Quantize rounds d to a multiple of q.
func (d Decimal) Quantize(q Decimal) Decimal {
	return d.QuantizeDown(q)
}

// Quantize rounds d to a multiple of q (towards zero).
func (d Decimal) QuantizeDown(q Decimal) Decimal {
	return Decimal(int64(d) / int64(q) * int64(q))
}

// QuantizeFloor rounds d down to a multiple of q (toward negative infinity).
func (d Decimal) QuantizeFloor(q Decimal) Decimal {
	qv := int64(q)
	v := int64(d)
	if v >= 0 {
		return Decimal(v / qv * qv)
	}
	rem := v % qv
	if rem == 0 {
		return d
	}
	return Decimal(v/qv*qv - qv)
}

// QuantizeCeil rounds d up to a multiple of q (toward positive infinity).
func (d Decimal) QuantizeCeil(q Decimal) Decimal {
	qv := int64(q)
	v := int64(d)
	if v <= 0 {
		return Decimal(v / qv * qv)
	}
	rem := v % qv
	if rem == 0 {
		return d
	}
	return Decimal(v/qv*qv + qv)
}

// QuantizeUp rounds d away from zero to a multiple of q.
func (d Decimal) QuantizeUp(q Decimal) Decimal {
	qv := int64(q)
	v := int64(d)
	rem := v % qv
	if rem == 0 {
		return d
	}
	if v > 0 {
		return Decimal(v/qv*qv + qv)
	}
	return Decimal(v/qv*qv - qv)
}

// QuantizeNearest rounds d to the nearest multiple of q (half away from zero).
func (d Decimal) QuantizeNearest(q Decimal) Decimal {
	qv := int64(q)
	v := int64(d)
	if v >= 0 {
		return Decimal((v + qv/2) / qv * qv)
	}
	return Decimal((v - qv/2) / qv * qv)
}
