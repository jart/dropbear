package main

import (
	"dropbear/broker/schwab"
	"dropbear/decimal"
	"dropbear/loggy"
	"encoding/json"
	"log"
	"time"
)

func main() {
	loggy.Init()
	loggy.AlsoLogToFile()
	client := schwab.NewClient()

	// start listening for order updates before placing the order
	updates := client.OrderUpdates()
	time.Sleep(1 * time.Second) // wait for streamer connection to establish

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
	log.Printf("created order %d", orderID)

	for update := range updates {
		pretty, _ := json.MarshalIndent(json.RawMessage(update.RawData), "  ", "  ")
		log.Printf("order %s: %s\n  %s", update.SchwabOrderID, update.Event, pretty)
	}
}
