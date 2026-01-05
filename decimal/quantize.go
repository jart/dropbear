package decimal

import "math"

// QuantizeTruncate rounds d toward zero to a multiple of q.
// This is consistent with IEEE 754 "round toward zero".
// Also known as rounding down in Python.
// Use this for order sizing.
func (d Decimal) QuantizeTruncate(q Decimal) Decimal {
	checkQuantum(q)
	return Decimal(int64(d) / int64(q) * int64(q))
}

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

// QuantizeNearest rounds d to the nearest multiple of q (Round Half Away From Zero).
// Use this for standard pricing and quoting.
func (d Decimal) QuantizeNearest(q Decimal) Decimal {
	checkQuantum(q)
	qv := int64(q)
	v := int64(d)

	quo := v / qv
	rem := v % qv

	absRem := rem
	if absRem < 0 {
		absRem = -absRem
	}

	threshold := (qv >> 1) + (qv & 1)

	if absRem >= threshold {
		if v < 0 {
			quo--
		} else {
			quo++
		}
	}

	// Final Safety Check: mulInt64
	// We are computing quo * qv.
	res := quo * qv

	// 1. Check if multiplication wrapped around
	if qv != 0 && res/qv != quo {
		panic("decimal overflow")
	}

	// 2. Edge case: MinInt64 * -1 (unlikely for quantum, but rigorous)
	if quo == math.MinInt64 && qv == -1 {
		panic("decimal overflow")
	}

	return Decimal(res)
}

func checkQuantum(q Decimal) {
	if q <= 0 {
		panic("illegal quantum")
	}
}
