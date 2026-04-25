package main

import (
	"dropbear/decimal"
	"dropbear/ds"
)

// Fee/rebate constants for equity DMA orders (always making).
var (
	kMakerRebatePerShare = decimal.Parse("0.0018")   // exchange rebate for providing liquidity
	kTafFeePerShare      = decimal.Parse("0.000195") // TAF fee (sells only)
	kCatFeePerTrade      = decimal.Parse("0.0003")   // CAT fee per trade
	kBrokerFeePerTrade   = decimal.Parse("0.0025")   // alpaca elite smart router fee per trade
)

// EstimateFee returns the estimated fee for a fill. Negative means net rebate.
// firstFill should be true only for the first fill of an order (CAT and broker
// fees are per-order, not per-fill).
func EstimateFee(side ds.Side, qty decimal.Decimal, firstFill bool) decimal.Decimal {
	fee := decimal.Zero
	if firstFill {
		fee = fee.Add(kCatFeePerTrade)
		fee = fee.Add(kBrokerFeePerTrade)
	}
	if side == ds.SideSell {
		fee = fee.Add(kTafFeePerShare.Mul(qty))
	}
	fee = fee.Sub(kMakerRebatePerShare.Mul(qty)) // rebate reduces fee
	return fee
}
