// foktest places a Fill-or-Kill order for a 0DTE SPX put at an absurdly low
// price to observe how Schwab reports FOK rejections via the streamer websocket.
//
// Usage:
//
//	go run ./broker/schwab/cmd/foktest
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/osi"
	"dropbear/ds/symbol"
	"dropbear/loggy"
)

func main() {
	loggy.Init()
	loggy.AlsoLogToFile()
	client := schwab.NewClient()

	// build today's 0DTE SPXW put OSI symbol at strike 6520
	now := clocky.Now()
	year, month, day := now.Date()
	sym := osi.Encode(symbol.MustParse("SPXW"), decimal.FromInt(6520), 'P', year, month, day)
	log.Printf("symbol: %s", sym)

	// start listening for order updates before placing the order
	updates := client.OrderUpdates()

	orderID, err := client.CreateOrder(&schwab.Order{
		OrderType:         schwab.OrderTypeLimit,
		Price:             decimal.Parse("0.05"),
		Duration:          schwab.DurationFillOrKill,
		Session:           schwab.SessionNormal,
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
		OrderLegCollection: []schwab.OrderLeg{{
			Instruction: schwab.InstructionBuyToOpen,
			Quantity:    decimal.One,
			Instrument: schwab.Instrument{
				Symbol:    sym,
				AssetType: schwab.AssetTypeOption,
			},
		}},
	})
	if err != nil {
		log.Fatalf("create order failed: %v", err)
	}
	log.Printf("FOK order created: id=%d", orderID)

	// log all events until we see a terminal state or timeout
	timeout := time.After(30 * time.Second)
	for {
		select {
		case event := <-updates:
			pretty, _ := json.MarshalIndent(json.RawMessage(event.RawData), "  ", "  ")
			log.Printf("order %s: %s\n  %s", event.SchwabOrderID, event.BaseEvent.EventType, pretty)
			// check for terminal events
			switch event.BaseEvent.EventType {
			case "OrderFillCompleted", "UROut":
				fmt.Println("terminal event received, exiting")
				return
			}
		case <-timeout:
			log.Printf("timeout after 30s, exiting")
			return
		}
	}
}
