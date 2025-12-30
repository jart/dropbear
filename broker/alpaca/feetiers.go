package alpaca

import "dropbear/decimal"

type FeeTier struct {
	Maker decimal.Decimal
	Taker decimal.Decimal
}

var FeeTiers = map[int]FeeTier{
	1: {Maker: decimal.Parse("0.0015"), Taker: decimal.Parse("0.0025")}, // default tier
	2: {Maker: decimal.Parse("0.0012"), Taker: decimal.Parse("0.0022")}, // 100k+ volume
	3: {Maker: decimal.Parse("0.0010"), Taker: decimal.Parse("0.0020")}, // 500k+ volume
	4: {Maker: decimal.Parse("0.0008"), Taker: decimal.Parse("0.0018")}, // 1m+ volume
	5: {Maker: decimal.Parse("0.0005"), Taker: decimal.Parse("0.0015")}, // 10m+ volume
	6: {Maker: decimal.Parse("0.0002"), Taker: decimal.Parse("0.0013")}, // 25m+ volume
	7: {Maker: decimal.Parse("0.0002"), Taker: decimal.Parse("0.0012")}, // 50m+ volume
	8: {Maker: decimal.Parse("0.0000"), Taker: decimal.Parse("0.0010")}, // 100m+ volume
}
