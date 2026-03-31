// numba defines arbitrary constants for convenience and readability
package decimal

const (
	NegOne = Decimal(-Scale)
	Two    = Decimal(Scale * 2)
	Three  = Decimal(Scale * 3)
	Five   = Decimal(Scale * 5)
	Seven  = Decimal(Scale * 7)
	Half   = Decimal(Scale / 2)
	Tenth  = Decimal(Scale / 10)
	Cent   = Decimal(Scale / 100) // 0.01
	Lot    = Decimal(Scale * 100) // 100
)
