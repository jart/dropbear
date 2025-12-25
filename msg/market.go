// Package msg defines message types for the actor system.
// Messages are passed between actors via mailboxes.
package msg

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
)

// BinanceTick is emitted by the Binance ingest actor when trade data arrives.
// Contains the latest trade price and time from Binance.
type BinanceTick struct {
	Time   clocky.Time
	Price  decimal.Decimal
	Trades []ds.Trade // raw trades for intensity calculation
}

// CoinbaseTick is emitted by the Coinbase ingest actor when order book changes.
// Contains bid/ask prices and the mid price.
type CoinbaseTick struct {
	Time clocky.Time
	Bid  decimal.Decimal
	Ask  decimal.Decimal
	Mid  decimal.Decimal
}

// PriceRange is emitted by the indicator actor with min/max range.
type PriceRange struct {
	Time clocky.Time
	Min  decimal.Decimal
	Max  decimal.Decimal
}

// IntensityUpdate is emitted when trading intensity (kappa) changes.
type IntensityUpdate struct {
	Time  clocky.Time
	Kappa decimal.Decimal // Avellaneda-Stoikov intensity parameter
}

// SpreadUpdate is emitted by the indicator actor with spread baseline.
type SpreadUpdate struct {
	Time      clocky.Time
	Spread    decimal.Decimal // current spread
	Baseline  decimal.Decimal // EMA baseline
	Deviation decimal.Decimal // spread - baseline
	Ready     bool            // true when EMA is warmed up
}

// MarketState aggregates all market data needed for signal generation.
// This is the input to the spread calculation actor.
type MarketState struct {
	Time          clocky.Time
	BinancePrice  decimal.Decimal
	CoinbasePrice decimal.Decimal
	CoinbaseMin   decimal.Decimal
	CoinbaseMax   decimal.Decimal
	SpreadEMA     decimal.Decimal
	Kappa         decimal.Decimal
	LastBinance   clocky.Time
	LastCoinbase  clocky.Time
}
