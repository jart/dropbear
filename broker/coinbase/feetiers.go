package coinbase

import "dropbear/decimal"

type FeeTier struct {
	Maker decimal.Decimal
	Taker decimal.Decimal
}

var FeeTiers = map[string]FeeTier{
	"Intro 1":    {Maker: decimal.Parse("0.0060000"), Taker: decimal.Parse("0.0120000")}, // default tier
	"Intro 2":    {Maker: decimal.Parse("0.0040000"), Taker: decimal.Parse("0.0080000")}, // $10k+ volume
	"Advanced 1": {Maker: decimal.Parse("0.0025000"), Taker: decimal.Parse("0.0050000")}, // $25k+ volume
	"Advanced 2": {Maker: decimal.Parse("0.0012500"), Taker: decimal.Parse("0.0025000")}, // $75k+ volume
	"Advanced 3": {Maker: decimal.Parse("0.0007500"), Taker: decimal.Parse("0.0015000")}, // $250k+ volume
	"VIP 2":      {Maker: decimal.Parse("0.0003750"), Taker: decimal.Parse("0.0007500")}, // $1m+ volume
	"VIP 3":      {Maker: decimal.Parse("0.0003000"), Taker: decimal.Parse("0.0006375")}, // $5m+ volume
	"VIP 4":      {Maker: decimal.Parse("0.0001875"), Taker: decimal.Parse("0.0004875")}, // $10m+ volume
	"VIP 5":      {Maker: decimal.Parse("0.0000750"), Taker: decimal.Parse("0.0003750")}, // $20m+ volume
	"VIP 6":      {Maker: decimal.Parse("0.0000000"), Taker: decimal.Parse("0.0002625")}, // $50m+ volume
	"VIP 7":      {Maker: decimal.Parse("0.0000000"), Taker: decimal.Parse("0.0001875")}, // $100m+ volume
	"VIP 8":      {Maker: decimal.Parse("0.0000000"), Taker: decimal.Parse("0.0001500")}, // $250m+ volume
}
