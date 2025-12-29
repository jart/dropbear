package decimal

import "math"

// QuantizeEven rounds d to the nearest multiple of q (Round Half To Even).
// This is statistically unbiased and known as Bankers' Rounding.
// Use this for algorithmic pricing to avoid drift.
func (d Decimal) QuantizeEven(q Decimal) Decimal {
	checkQuantum(q)
	qv := int64(q)
	v := int64(d)

	// 1. Initial Quotient and Remainder
	quo := v / qv
	rem := v % qv

	// 2. Safe Threshold Calculation
	absRem := rem
	if absRem < 0 {
		absRem = -absRem
	}

	// Threshold = ceil(|q| / 2)
	// (qv >> 1) + (qv & 1) avoids overflow when qv is MaxInt64
	threshold := (qv >> 1) + (qv & 1)

	// 3. Compare Remainder to Threshold
	var comp int8 // -1: less, 0: exact half, 1: more
	if absRem > threshold {
		comp = 1
	} else if absRem == threshold {
		if qv&1 == 1 {
			comp = 1 // Odd quantum means exact half is impossible -> strictly greater
		} else {
			comp = 0 // Exact half
		}
	} else {
		comp = -1
	}

	// 4. Rounding Decision
	// Round away from zero if:
	// - Remainder > Half (comp > 0)
	// - Remainder == Half AND Quotient is Odd (comp == 0 && quo is odd)
	if comp > 0 || (comp == 0 && (quo&1) != 0) {
		if v < 0 {
			// Rounding down (more negative)
			if quo == math.MinInt64 {
				panic("decimal overflow")
			}
			quo--
		} else {
			// Rounding up (more positive)
			if quo == math.MaxInt64 {
				panic("decimal overflow")
			}
			quo++
		}
	}

	// 5. Final Safety Check: Multiplication Overflow
	// We are computing res = quo * qv
	res := quo * qv

	if qv != 0 && res/qv != quo {
		panic("decimal overflow")
	}

	// Edge case: MinInt64 * -1 (if quantum was negative)
	if quo == math.MinInt64 && qv == -1 {
		panic("decimal overflow")
	}

	return Decimal(res)
}
