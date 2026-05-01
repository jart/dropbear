package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/clocky"
	"dropbear/decimal"
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

type Result struct {
	PnL    decimal.Decimal
	Fees   decimal.Decimal
	Net    decimal.Decimal
	Equity decimal.Decimal
	Shares decimal.Decimal
	Fills  int
	Log    string
}

// State tracks all state for a single symbol.
type State struct {
	symbol        symbol.Symbol
	asset         *alpaca.Asset
	quote         *sip.Quote
	config        Config
	halt          bool
	disabled      bool
	position      decimal.Decimal // negative if short
	cooldownUntil clocky.Time
	buyPrice      decimal.Decimal
	sellPrice     decimal.Decimal
	buyPrice2     decimal.Decimal
	sellPrice2    decimal.Decimal

	// our pending orders
	buyOrderID         string
	sellOrderID        string
	buyClientOrderID   string
	sellClientOrderID  string
	buyClientOrderID2  string
	sellClientOrderID2 string

	// LULD state
	luldLower    decimal.Decimal // current lower price band (zero if no band)
	luldUpper    decimal.Decimal // current upper price band (zero if no band)
	luldAt       clocky.Time    // when the band was last published
	lastTrade    decimal.Decimal // most recent trade price
	lastTradeAt  clocky.Time    // when the most recent trade happened
	imbPrice     decimal.Decimal // most recent imbalance price during halt
	scoopEntryAt clocky.Time    // when we entered the position
	scoopTarget  decimal.Decimal // price we want to exit at
	scoopShort   bool            // true if this is a short position (limit up)

	// P&L tracking
	costBasis    decimal.Decimal // total cost of current position (signed)
	realizedPnL  decimal.Decimal // cumulative realized P&L
	totalBought  decimal.Decimal // total shares bought
	totalSold    decimal.Decimal // total shares sold
	totalCostIn  decimal.Decimal // total dollars spent buying
	totalCostOut decimal.Decimal // total dollars received selling
	totalFees    decimal.Decimal // cumulative fees (negative = net rebate)
}

func (st *State) HasBeenTraded() bool {
	return st.buyOrderID != "" || st.sellOrderID != "" || st.position.Sign() != 0 || st.realizedPnL.Sign() != 0
}
