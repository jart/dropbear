package decimal

import "math"

// Short represents a 32-bit fixed-point decimal.
// Convert this type back to Decimal for arithmetic.
type Short int32

const (
	ShortScale = 1_000
	ShortMax   = Short(math.MaxInt32) // +2'147'483.647
	ShortMin   = Short(math.MinInt32) // -2'147'483.648
	shortRatio = Scale / ShortScale
)

// Decimal converts a 32-bit Short to a 64-bit Decimal.
func (s Short) Decimal() Decimal {
	return Decimal(s) * shortRatio
}

// Short converts a 64-bit Decimal to a 32-bit Short.
// This method panics if d is too large to become Short.
// Extra decimal places are removed with banker's rounding.
func (d Decimal) Short() Short {
	x := int64(d)

	// get sign and absolute value
	xm := x >> 63
	ux := uint64((x ^ xm) - xm)

	// unsigned division
	quo := ux / shortRatio
	rem := ux % shortRatio

	// Banker's Rounding (Round Half To Even)
	if (rem<<1)+(quo&1) > shortRatio {
		quo++
	}

	// overflow check
	if quo > uint64(ShortMax)+(uint64(xm)&1) {
		panic("decimal overflow")
	}

	// apply sign
	return Short(int32((quo ^ uint64(xm)) - uint64(xm)))
}
