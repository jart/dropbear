package main

import (
	"dropbear/broker/schwab"
	"dropbear/decimal"
)

func main() {
	client := schwab.NewClient()
	orderID, err := client.CreateOrder(&schwab.Order{
		OrderType: schwab.OrderTypeLimit,
		Price:     decimal.Parse("28.47"),
		Duration:  schwab.DurationDay,
		Session:   schwab.SessionSeamless,
		OrderLegCollection: []schwab.OrderLeg{
			{
				Instruction: schwab.InstructionBuy,
				Quantity:    decimal.FromInt(1),
				Instrument: schwab.Instrument{
					Symbol: "T",
					Type:   schwab.AssetTypeEquity,
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	println("Created order with ID", orderID)
}
