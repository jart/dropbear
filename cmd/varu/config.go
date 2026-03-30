package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

type Config struct {
	Eval         decimal.Decimal
	Spread       decimal.Decimal
	Sigmas       decimal.Decimal
	Demand       decimal.Decimal
	WeightRisk   decimal.Decimal
	WeightDelta  decimal.Decimal
	WeightPayoff decimal.Decimal
	Cooldown     clocky.Duration
	Patience     clocky.Duration
	Think        clocky.Duration
	Strategies   map[string]bool
	Floor        float64
	Budget       float64
	Prune        float64
	Listen       string
	StartOfDay   int
	FullRiskTime int
	StopTrading  int
	MaxPending   int
	EOD          bool
}
