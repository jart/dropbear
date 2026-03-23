package main

import (
	"encoding/json"
	"log"
	"os"

	"dropbear/broker/schwab"
	"dropbear/decimal"
	"dropbear/loggy"
)

func main() {
	loggy.Init()
	loggy.AlsoLogToFile()
	client := schwab.NewClient()
	updates := client.OrderUpdates()

	orderID, err := client.CreateOrder(&schwab.Order{
		OrderType:         schwab.OrderTypeLimit,
		Price:             decimal.Parse("0.50"),
		Duration:          schwab.DurationDay,
		Session:           schwab.SessionNormal,
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
		OrderLegCollection: []schwab.OrderLeg{
			{
				Instruction: schwab.InstructionBuyToOpen,
				Quantity:    decimal.FromInt(1),
				Instrument: schwab.Instrument{
					Symbol:    "SPXW  260305P06720000",
					AssetType: schwab.AssetTypeOption,
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
		pretty, _ := json.MarshalIndent(event, "  ", "  ")
		log.Printf("order %s: %s\n  %s", event.SchwabOrderID, event.BaseEvent.EventType, pretty)
	}
}
