package main

import (
	"dropbear/decimal"
	"log"
)

var (
	holdings map[string]decimal.Decimal
)

func InitHoldings() {
	holdings = make(map[string]decimal.Decimal)
	acct, err := schwabClient.GetAccount()
	if err != nil {
		log.Printf("warning: failed to fetch positions: %v", err)
	} else {
		for _, pos := range acct.SecuritiesAccount.Positions {
			qty := pos.LongQuantity.Sub(pos.ShortQuantity)
			if !qty.IsZero() {
				holdings[pos.Instrument.Symbol] = qty
			}
		}
	}
}
