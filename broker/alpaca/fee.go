package alpaca

import (
	"dropbear/decimal"
	"dropbear/ds"
)

var (
	BrokerFeePerShare = decimal.Parse("0.0020")   // alpaca elite smart router fee
	TakerFeePerShare  = decimal.Parse("0.0020")   // exchange fee for taking liquidity
	MakerFeePerShare  = decimal.Parse("-0.0018")  // exchange rebate for providing liquidity
	SECFeePerMillion  = decimal.Parse("20.60")    // SEC fee per $1M proceeds (sells only)
	TAFFeePerShare    = decimal.Parse("0.000195") // TAF fee (sells only)
	CATFeePerTrade    = decimal.Parse("0.0003")   // CAT fee per trade
)

// EstimateFee returns the estimated fee for a fill.
func EstimateFee(side ds.Side, qty, price decimal.Decimal, firstFill, marketable bool) decimal.Decimal {
	fee := decimal.Zero
	if firstFill {
		fee = fee.Add(CATFeePerTrade)
	}
	fee = fee.Add(BrokerFeePerShare.Mul(qty))
	if side == ds.SideSell {
		fee = fee.Add(TAFFeePerShare.Mul(qty))
		fee = fee.Add(price.Mul(qty).Mul(SECFeePerMillion).DivInt(1_000_000))
	}
	if marketable {
		fee = fee.Add(TakerFeePerShare.Mul(qty))
	} else {
		fee = fee.Add(MakerFeePerShare.Mul(qty))
	}
	return fee
}
