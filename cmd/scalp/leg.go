package main

import (
	"dropbear/decimal"
	"dropbear/options"
	"dropbear/symbol"
	"fmt"
)

type Leg struct {
	Security options.Security
	Quantity decimal.Decimal
	Filled   bool
}

func (leg *Leg) String() string {
	return fmt.Sprintf("%s %s", leg.Quantity, leg.Security.String())
}

func (leg *Leg) EstimateFee(marketable bool) decimal.Decimal {
	qty := leg.Quantity.Abs()
	switch s := leg.Security.(type) {
	case *options.Option:
		switch s.Symbol {
		case symbol.SPXW, symbol.RUTW, symbol.NDX:
			return kFeePerOptionsContractSPXW.Mul(qty)
		default:
			return kFeePerOptionsContract.Mul(qty)
		}
	case *options.Equity:
		fee := kTafFeePerShare.Mul(qty)
		if marketable {
			fee = fee.Add(kExchangeTakerFeePerShare.Mul(qty))
		} else {
			fee = fee.Add(kExchangeMakerFeePerShare.Mul(qty))
		}
		return fee
	default:
		panic("unknown security type")
	}
}
