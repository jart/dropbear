package main

import (
	"dropbear/broker/schwab"
	"dropbear/decimal"
)

func main() {
	client := schwab.NewClient()
	client.CreateOrder(&schwab.Order{
		OrderType:                schwab.OrderTypeLimit,
		Price:                    decimal.Parse("30.00"),
		Duration:                 schwab.DurationDay,
		Session:                  schwab.SessionAM,
		OrderStrategyType:        schwab.OrderStrategyTypeSingle,
		OrderLegCollection: []schwab.OrderLeg{
			{
				Instruction: schwab.InstructionBuyToOpen,
				Quantity:    decimal.FromInt(1),
				Instrument: schwab.Instrument{
					Symbol: "T",
					Type:   schwab.AssetTypeEquity,
				},
			},
		},
	})
}
