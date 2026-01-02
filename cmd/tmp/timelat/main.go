package main

import (
	"dropbear/broker/binance"
	"dropbear/clocky"
	"log"
)

func main() {
	client := binance.NewClient()
	for i := range 3 {
		start := clocky.Now()
		_, err := client.GetTime()
		latency := clocky.Since(start)
		if err != nil {
			log.Printf("request %d error: %v", i+1, err)
		} else {
			log.Printf("request %d latency: %v", i+1, latency)
		}
	}
}
