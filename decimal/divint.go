package decimal

import "math"

// DivInt divides d by n.
// This uses Bankers' Rounding to break ties.
// Panics on overflow (Min / -1) or division by zero.
func (d Decimal) DivInt(n int) Decimal {
	return d.DivInt64(int64(n))
}

// DivInt64 divides d by n.
// This uses Bankers' Rounding to break ties.
// Panics on overflow (Min / -1) or division by zero.
func (d Decimal) DivInt64(n int64) Decimal {
	x, y := int64(d), int64(n)

	// check for overflow
	if x == math.MinInt64 && y == -1 {
		panic("decimal overflow")
	}

	// get sign and absolute values
	xm := x >> 63
	ym := y >> 63
	ux := uint64((x ^ xm) - xm)
	uy := uint64((y ^ ym) - ym)
	sm := (x ^ y) >> 63

	// unsigned division
	quo := ux / uy
	rem := ux % uy

	// Banker's Rounding (Round Half To Even)
	// We want to increment quo if:
	// 1. rem > uy/2
	// 2. rem == uy/2 AND quo is odd (exact half case)
	//
	// This is mathematically equivalent to:
	// (2 * rem) + (quo & 1) > uy
	//
	// Overflow safety: rem < uy <= 2^63, so 2*rem + 1 <= 2^64 - 1.
	if (rem<<1)+(quo&1) > uy {
		quo++
	}

	// apply sign using branchless arithmetic
	return Decimal(int64((quo ^ uint64(sm)) - uint64(sm)))
}
