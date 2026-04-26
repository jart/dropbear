package cboe

import "dropbear/decimal"

const (
	k1     = decimal.Decimal(1 * decimal.Scale)
	k10    = decimal.Decimal(10 * decimal.Scale)
	k40    = decimal.Decimal(40 * decimal.Scale)
	k100   = decimal.Decimal(100 * decimal.Scale)
	k250   = decimal.Decimal(250 * decimal.Scale)
	k1000  = decimal.Decimal(1000 * decimal.Scale)
	k10000 = decimal.Decimal(10000 * decimal.Scale)
)

// LotSize returns number of shares in round lot.
func LotSize(price decimal.Decimal) decimal.Decimal {
	if price.Cmp(k10000) >= 0 {
		return k1
	} else if price.Cmp(k1000) >= 0 {
		return k10
	} else if price.Cmp(k250) >= 0 {
		return k40
	} else {
		return k100
	}
}
