package decimal

// Sqrt returns the square root of d.
func (d Decimal) Sqrt() Decimal {
	if d <= 0 {
		return 0
	}
	// Newton's method: x = (x + d/x) / 2
	x := d
	for range 20 {
		next := x.Add(d.Div(x)).DivInt(2)
		if next.Cmp(x) == 0 {
			break
		}
		x = next
	}
	return x
}
