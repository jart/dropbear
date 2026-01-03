package main

import (
	"dropbear/broker/binanceusd"
	"dropbear/clocky"
	"fmt"
)

func main() {
	var del clocky.Duration
	avg, cnt := 0.0, 1.0
	bes, wor := clocky.Minute, clocky.Microsecond
	for tick := range binanceusd.MarketData("BTCUSDT", binanceusd.NewClient()) {
		for i := range tick.TradeCount() {
			trade := tick.Trade(i)
			del = tick.Time().Sub(trade.Time)
			bes = min(bes, del)
			wor = max(wor, del)
			avg += (float64(del) - avg) / cnt
			cnt++
		}
		fmt.Printf("\r\033[K%s (avg %s, best %s, worst %s, n=%.0f)", del, clocky.Duration(avg), bes, wor, cnt)
	}
}
