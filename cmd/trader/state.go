package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/decimal"
)

// State tracks all state for a single symbol.
type State struct {
	symbol   string
	asset    *alpaca.Asset
	quote    *sip.Quote
	position decimal.Decimal // negative if short

	// our pending orders
	buyClientOrderID  string
	sellClientOrderID string

	// P&L tracking
	costBasis    decimal.Decimal // total cost of current position (signed)
	realizedPnL  decimal.Decimal // cumulative realized P&L
	totalBought  decimal.Decimal // total shares bought
	totalSold    decimal.Decimal // total shares sold
	totalCostIn  decimal.Decimal // total dollars spent buying
	totalCostOut decimal.Decimal // total dollars received selling
	totalFees    decimal.Decimal // cumulative fees (negative = net rebate)
}

func (st *State) Active() bool {
	return !st.position.IsZero() || st.buyClientOrderID != "" || st.sellClientOrderID != ""
}
