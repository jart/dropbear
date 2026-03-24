package decimal

import "math"

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
