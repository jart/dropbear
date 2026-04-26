package main

import (
	"dropbear/decimal"
	"dropbear/ds"
)

// Fee/rebate constants for equity DMA orders (always making).
var (
	kTakerFeePerShare  = decimal.Parse("0.0020")   // exchange fee for taking liquidity
	kMakerFeePerShare  = decimal.Parse("-0.0018")  // exchange rebate for providing liquidity
	kSECFeePerMillion  = decimal.Parse("20.60")    // SEC fee per $1M proceeds (sells only)
	kTAFFeePerShare    = decimal.Parse("0.000195") // TAF fee (sells only)
	kCATFeePerTrade    = decimal.Parse("0.0003")   // CAT fee per trade
	kBrokerFeePerTrade = decimal.Parse("0.0025")   // alpaca elite smart router fee per trade
)

// EstimateFee returns the estimated fee for a fill. Negative means net rebate.
// firstFill should be true only for the first fill of an order (CAT and broker
// fees are per-order, not per-fill).
func EstimateFee(side ds.Side, qty, price decimal.Decimal, firstFill bool) decimal.Decimal {
	fee := decimal.Zero
	if firstFill {
		fee = fee.Add(kCATFeePerTrade)
		fee = fee.Add(kBrokerFeePerTrade)
	}
	if side == ds.SideSell {
		fee = fee.Add(kTAFFeePerShare.Mul(qty))
		fee = fee.Add(price.Mul(qty).Mul(kSECFeePerMillion).DivInt(1_000_000))
	}
	fee = fee.Add(kMakerFeePerShare.Mul(qty))
	return fee
}
