package decimal

import (
	"math"
	"math/bits"
)

// MulInt multiplies d by n.
// Panics on overflow.
func (d Decimal) MulInt(n int) Decimal {
	x, y := int64(d), int64(n)

	// get sign and absolute values
	xm := x >> 63
	ym := y >> 63
	ux := uint64((x ^ xm) - xm)
	uy := uint64((y ^ ym) - ym)
	sm := (x ^ y) >> 63

	// 128-bit multiply
	hi, lo := bits.Mul64(ux, uy)

	// overflow check: result must fit in int64
	if hi != 0 || lo > math.MaxInt64 {
		// allow MinInt64 only if sign is negative
		if sm == -1 && hi == 0 && lo == 1<<63 {
			return Decimal(math.MinInt64)
		}
		panic("decimal overflow")
	}

	// apply sign
	return Decimal(int64((lo ^ uint64(sm)) - uint64(sm)))
}
