package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/indicators"
	"dropbear/symbol"
)

type Config struct {
	venue  alpaca.OrderDestination
	target decimal.Decimal
	qty    decimal.Decimal
	spread decimal.Decimal
	drift  decimal.Decimal
	greed  decimal.Decimal
}

type SymbolEntry struct {
	Symbol symbol.Symbol
	Config Config
}

type Result struct {
	PnL    decimal.Decimal
	Fees   decimal.Decimal
	Net    decimal.Decimal
	Equity decimal.Decimal
	Fills  int
	Shares decimal.Decimal
	Log    string
}

// State tracks all state for a single symbol.
type State struct {
	symbol        symbol.Symbol
	asset         *alpaca.Asset
	quote         *sip.Quote
	config        Config
	halt          bool
	position      decimal.Decimal // negative if short
	cooldownUntil clocky.Time
	buyPrice      decimal.Decimal
	sellPrice     decimal.Decimal
	buyPrice2     decimal.Decimal
	sellPrice2    decimal.Decimal
	ema           *indicators.WWMA // exponential moving average of midpoint
	buyGreed      decimal.Decimal  // widens bid after consecutive buy fills
	sellGreed     decimal.Decimal  // widens ask after consecutive sell fills

	// our pending orders
	buyOrderID         string
	sellOrderID        string
	buyClientOrderID   string
	sellClientOrderID  string
	buyClientOrderID2  string
	sellClientOrderID2 string

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
