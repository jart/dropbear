package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/indicators"
	"dropbear/symbol"
)

// State tracks all state for a single symbol.
type State struct {
	symbol   symbol.Symbol
	asset    *alpaca.Asset
	quote    *sip.Quote
	halt     bool
	greed    decimal.Decimal
	position decimal.Decimal // negative if short

	// price moving average
	pricema   *indicators.WWMA
	nextprice clocky.Time

	// volatility tracking
	minTradePrice  *indicators.Min
	maxTradePrice  *indicators.Max
	lastTradePrice decimal.Decimal

	// volume tracking
	volma      *indicators.WWMA // smoothed minute bar volume
	volsum     decimal.Decimal  // smoothed minute bar volume
	highVolume bool             // in top percentile by volume

	// iso flow tracking
	isoNetFlow decimal.Decimal // net ISO shares: positive = buyer, negative = seller

	// our pending orders
	buyOrderID        string
	sellOrderID       string
	buyClientOrderID  string
	sellClientOrderID string
	orderCreatedTime  clocky.Time // only cancelable if positive
	cooldownUntil     clocky.Time // don't trade until this time

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
