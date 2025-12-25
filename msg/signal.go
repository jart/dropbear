package msg

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
)

// Disposition indicates where price is in the recent range.
type Disposition int

const (
	DispositionNormal Disposition = iota
	DispositionCheap
	DispositionTooCheap
	DispositionExpensive
	DispositionTooExpensive
)

// SpreadSignal is emitted when a trading opportunity is detected.
type SpreadSignal struct {
	Time        clocky.Time
	Side        ds.Side
	Deviation   decimal.Decimal // spread deviation from baseline
	Disposition Disposition
	BuySpread   decimal.Decimal // adjusted spread threshold for buying
	SellSpread  decimal.Decimal // adjusted spread threshold for selling
}

// TradeIntent is emitted by the spread actor when it wants to trade.
// This goes to the risk actor for validation.
type TradeIntent struct {
	Time      clocky.Time
	Side      ds.Side
	Quantity  decimal.Decimal
	Price     decimal.Decimal // expected fill price
	Deviation decimal.Decimal // spread deviation that triggered this
}

// TradeResult is emitted after a trade is executed.
type TradeResult struct {
	Time      clocky.Time
	Side      ds.Side
	Quantity  decimal.Decimal
	Price     decimal.Decimal // actual fill price
	Slippage  decimal.Decimal // price slippage
	Success   bool
	Error     string
	OrderTime clocky.Duration // time to place order
	FillTime  clocky.Duration // time to get fill confirmation
}

// InventoryState is emitted by the accounting actor.
type InventoryState struct {
	Time       clocky.Time
	Quantity   decimal.Decimal
	CostBasis  decimal.Decimal
	BuyVolume  decimal.Decimal
	SellVolume decimal.Decimal
	WinCount   int
	LossCount  int
}

// HealthScore is computed from inventory balance and win rate.
type HealthScore struct {
	Time    clocky.Time
	Balance decimal.Decimal // min(buy,sell)/max(buy,sell)
	WinRate decimal.Decimal // wins/(wins+losses)
	Score   decimal.Decimal // balance * winRate
}

// GreedFactor is computed from inventory imbalance.
type GreedFactor struct {
	Time       clocky.Time
	Imbalance  decimal.Decimal // (inventory - target) / target
	Greed      decimal.Decimal // exp(imbalance * skew)
	BuySpread  decimal.Decimal // spread * greed
	SellSpread decimal.Decimal // spread / greed
}
