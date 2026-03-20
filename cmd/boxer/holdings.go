package main

import (
	"dropbear/decimal"
	"log"
)

var (
	gHoldings map[string]decimal.Decimal
)

func InitHoldings() {
	gHoldings = make(map[string]decimal.Decimal)
	acct, err := gSchwabClient.GetAccount()
	if err != nil {
		log.Printf("warning: failed to fetch positions: %v", err)
	} else {
		for _, pos := range acct.SecuritiesAccount.Positions {
			qty := pos.LongQuantity.Sub(pos.ShortQuantity)
			if !qty.IsZero() {
				gHoldings[pos.Instrument.Symbol] = qty
			}
		}
	}
}
