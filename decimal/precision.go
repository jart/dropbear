package decimal

// Precision returns the number of decimal places in this value.
// Useful for determining formatting precision from an increment like 0.01 -> 2.
// For example Decimal.Parse("1.2034").Format(Decimal.Parse("0.01").Precision()) -> "1.20"
// Whereas Parse("1.2034").Quantize(Decimal.Parse("0.01")).String() -> "1.2"
// Whereas Decimal.Parse("1.2034").String() -> "1.2034"
func (d Decimal) Precision() int {
	v := int64(d)
	if v < 0 {
		v = -v
	}
	n := Places
	for n > 0 && v%10 == 0 {
		v /= 10
		n--
	}
	return n
}
