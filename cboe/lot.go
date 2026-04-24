package cboe

import "dropbear/decimal"

var (
	k10000 = decimal.Parse("10000")
	k1000  = decimal.Parse("1000")
	k250   = decimal.Parse("250")
	k100   = decimal.Parse("100")
	k40    = decimal.Parse("40")
)

// LotSize returns number of shares in round lot.
func LotSize(price decimal.Decimal) decimal.Decimal {
	if price.Cmp(k10000) >= 0 {
		return decimal.One
	} else if price.Cmp(k1000) >= 0 {
		return decimal.Ten
	} else if price.Cmp(k250) >= 0 {
		return k40
	} else {
		return k100
	}
}
