package main

import (
	"dropbear/decimal"
	"log"
)

func InitHoldings() {
	acct, err := gSchwabClient.GetAccount()
	if err != nil {
		log.Printf("warning: failed to fetch positions: %v", err)
	} else {
		for _, pos := range acct.SecuritiesAccount.Positions {
			qty := pos.LongQuantity.Sub(pos.ShortQuantity)
			if !qty.IsZero() {
				gHoldings.Put(pos.Instrument.Symbol, qty)
			}
		}
	}
}

func GetHoldings(sym string) decimal.Decimal {
	have, _ := gHoldings.Get(sym)
	return have
}

func AddToHoldings(sym string, qty decimal.Decimal) {
	have, _ := gHoldings.Get(sym)
	gHoldings.Put(sym, have.Add(qty))
}
