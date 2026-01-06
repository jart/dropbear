package decimal

// Truncate returns the integer part of d (Toward Zero).
// For example:  1.5 -> 1, -1.5 -> -1.
func (d Decimal) Truncate() Decimal {
	return d.QuantizeTruncate(One)
}

// RoundNearest returns the nearest integer (Half Away From Zero).
// This has a statistical bias but is often preferred in general usage.
// For example: 2.5 -> 3, -2.5 -> -3.
func (d Decimal) RoundNearest() Decimal {
	return d.QuantizeNearest(One)
}

// RoundEven returns the nearest integer (Half To Even).
// This is statistically unbiased and known as Bankers' Rounding.
// For example: 2.5 -> 2, -2.5 -> -2.
func (d Decimal) RoundEven() Decimal {
	return d.QuantizeEven(One)
}

// RoundAway returns the nearest integer (Half Away From Zero).
// For example: 1.5 -> 2, -1.5 -> -2.
func (d Decimal) RoundAway() Decimal {
	return d.QuantizeAway(One)
}

// Floor returns the greatest integer value less than or equal to d.
// For example: 1.5 -> 1, -1.5 -> -2
func (d Decimal) Floor() Decimal {
	return d.QuantizeFloor(One)
}

// Ceil returns the least integer value greater than or equal to d.
// For example: 1.5 -> 2, -1.5 -> -1
func (d Decimal) Ceil() Decimal {
	return d.QuantizeCeil(One)
}

// RoundToNearest returns the nearest integer (Half Away From Zero).
// This has a statistical bias but is often preferred in general usage.
// For example: 2.5 -> 3, -2.5 -> -3.
func (d Decimal) RoundToNearest(precision int) Decimal {
	return d.QuantizeNearest(Decimal(pow10[9-precision]))
}

// RoundToEven returns the nearest integer (Half To Even).
// This is statistically unbiased and known as Bankers' Rounding.
// For example: 2.5 -> 2, -2.5 -> -2.
func (d Decimal) RoundToEven(precision int) Decimal {
	return d.QuantizeEven(Decimal(pow10[9-precision]))
}

// FloorTo returns the greatest integer value less than or equal to d.
// For example: 1.5 -> 1, -1.5 -> -2
func (d Decimal) FloorTo(precision int) Decimal {
	return d.QuantizeFloor(Decimal(pow10[9-precision]))
}

// CeilTo returns the least integer value greater than or equal to d.
// For example: 1.5 -> 2, -1.5 -> -1
func (d Decimal) CeilTo(precision int) Decimal {
	return d.QuantizeCeil(Decimal(pow10[9-precision]))
}

// TruncateTo returns the integer part of d (Toward Zero).
// For example:  1.5 -> 1, -1.5 -> -1.
func (d Decimal) TruncateTo(precision int) Decimal {
	return d.QuantizeTruncate(Decimal(pow10[9-precision]))
}
