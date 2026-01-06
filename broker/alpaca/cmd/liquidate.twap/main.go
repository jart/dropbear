package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"log"
)

func main() {
	loggy.Init()
	client := alpaca.NewClient()
	positions, err := client.GetPositions()
	if err != nil {
		loggy.Fatalf("getting positions: %v", err)
	}
	for _, pos := range positions {
		if pos.AssetClass != alpaca.AssetClassUSEquity {
			continue
		}
		_, err := client.CreateOrder(&alpaca.OrderRequest{
			Symbol:      pos.Symbol,
			Side:        ds.SideSell,
			Qty:         pos.Qty,
			LimitPrice:  pos.CurrentPrice.Mul(decimal.Parse("0.99")).QuantizeNearest(decimal.Cent),
			Type:        alpaca.OrderTypeLimit,
			TimeInForce: alpaca.TimeInForceDay,
			AdvancedInstructions: &alpaca.AdvancedInstructions{
				Algorithm:     alpaca.OrderAlgorithmTWAP,
				MaxPercentage: decimal.Parse("0.2"),
			},
		})
		if err != nil {
			log.Printf("error liquidating %s: %v", pos.Symbol, err)
		}
		log.Printf("liquidation order placed for %s qty %s", pos.Symbol, pos.Qty)
	}
}
