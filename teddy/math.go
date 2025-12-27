package teddy

import "dropbear/decimal"

// add performs checked unlocked atomic addition of x into dest.
func add(dest *decimal.Decimal, x decimal.Decimal) {
	dest.Store(dest.Load().Add(x))
}

// sub performs checked unlocked atomic subtraction of x from dest.
func sub(dest *decimal.Decimal, x decimal.Decimal) {
	dest.Store(dest.Load().Sub(x))
}
