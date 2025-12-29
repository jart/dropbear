package decimal

import "math"

// QuantizeAway rounds d away from zero to a multiple of q.
// Use this for conservative margin calculations.
// Also known as rounding up in IEEE 754.
func (d Decimal) QuantizeAway(q Decimal) Decimal {
	checkQuantum(q)
	qv := int64(q)
	v := int64(d)

	quo := v / qv
	rem := v % qv

	if rem != 0 {
		if v > 0 {
			// Rounding away (up)
			if quo == math.MaxInt64 {
				panic("decimal overflow")
			}
			quo++
		} else {
			// Rounding away (down/more negative)
			if quo == math.MinInt64 {
				panic("decimal overflow")
			}
			quo--
		}
	}

	// Final Safety Check: Check for multiplication overflow
	// We are computing res = quo * qv
	res := quo * qv

	if qv != 0 && res/qv != quo {
		panic("decimal overflow")
	}

	// Edge case check for MinInt64 * -1 (if qv is negative)
	if quo == math.MinInt64 && qv == -1 {
		panic("decimal overflow")
	}

	return Decimal(res)
}
