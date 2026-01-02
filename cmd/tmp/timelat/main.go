package main

import (
	"dropbear/broker/binance"
	"dropbear/clocky"
	"log"
)

func main() {
	client := binance.NewClient()
	for range 2 {
		start := clocky.Now()
		_, err := client.GetTime()
		latency := clocky.Since(start)
		if err != nil {
			log.Printf("error: %v", err)
		} else {
			log.Printf("latency: %v", latency)
		}
	}
}
