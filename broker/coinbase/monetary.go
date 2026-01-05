package coinbase

import "dropbear/decimal"

type MonetaryAmount struct {
	Value    decimal.Decimal `json:"value"`
	Currency string          `json:"currency"`
}
