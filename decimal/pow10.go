package decimal

var pow10 = []int64{
	1,
	10,
	100,
	1000,
	10000,
	100000,
	1000000,
	10000000,
	100000000,
	1000000000,
	10000000000,
	100000000000,
	1000000000000,
	10000000000000,
	100000000000000,
	1000000000000000,
	10000000000000000,
	100000000000000000,
	1000000000000000000,
}

// Precision returns a Decimal with the specified number of decimal places.
// For example, Precision(0) returns 1.0, Precision(2) returns 0.01.
// Negative precision values are undefined.
func Precision(p int) Decimal {
	if p < 0 {
		panic("negative precision undefined")
	}
	if p > Places {
		p = Places
	}
	return Decimal(pow10[Places-p])
}
