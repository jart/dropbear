// This example demonstrates how to sell a combo. This is when you sell a call and buy a put
// at the same strike. It creates a synthetic short position that's very much like selling a
// futures contract, in the sense that there's unlimited upside risk and unlimited dowmnside
// too (if the XSP underlying goes up). Since we're trading XSP we get the tax advantages of
// futures too (60/40 capital gains treatment).
package main

import (
	"encoding/json"
	"log"
	"os"

	"dropbear/broker/schwab"
	"dropbear/decimal"
	"dropbear/ds/osi"
	"dropbear/ds/symbol"
	"dropbear/loggy"
)

func main() {
	loggy.Init()
	loggy.AlsoLogToFile()
	client := schwab.NewClient()
	updates := client.OrderUpdates()

	orderID, err := client.CreateOrder(&schwab.Order{
		OrderType:         schwab.OrderTypeNetCredit,
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
		Price:             decimal.Parse("0.70"),
		OrderLegCollection: []schwab.OrderLeg{
			{
				Instruction: schwab.InstructionSellToOpen,
				Quantity:    decimal.FromInt(1),
				Instrument: schwab.Instrument{
					AssetType: schwab.AssetTypeOption,
					Symbol:    osi.Encode(symbol.MustParse("XSP"), decimal.FromInt(658), 'C', 2026, 3, 23),
				},
			},
			{
				Instruction: schwab.InstructionBuyToOpen,
				Quantity:    decimal.FromInt(1),
				Instrument: schwab.Instrument{
					AssetType: schwab.AssetTypeOption,
					Symbol:    osi.Encode(symbol.MustParse("XSP"), decimal.FromInt(658), 'P', 2026, 3, 23),
				},
			},
		},
	})
	if err != nil {
		log.Printf("got error creating order: %v", err)
		os.Exit(1)
	}
	log.Printf("created order %d", orderID)

	for event := range updates {
		pretty, _ := json.MarshalIndent(json.RawMessage(event.RawData), "  ", "  ")
		log.Printf("order %s: %s\n  %s", event.SchwabOrderID, event.BaseEvent.EventType, pretty)
	}
}
