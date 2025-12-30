package main

import (
	"dropbear/broker/binance"
	"dropbear/broker/binanceusd"
	"dropbear/broker/coinbase"
	"dropbear/decimal"
	"fmt"
	"log"
)

func main() {
	avg, cnt := 0.0, 1.0
	bes, wor := decimal.Parse("99999999"), decimal.Zero
	spot := binance.MarketData("BTCUSDT", binance.NewClient())
	coin := coinbase.MarketData("BTC-USD", coinbase.NewClient())
	futures := binanceusd.MarketData("BTCUSDT", binanceusd.NewClient())
	spotPrice, futuresPrice, coinbasePrice := decimal.Zero, decimal.Zero, decimal.Zero
	for {
		select {
		case tick := <-coin:
			for _, trade := range tick.Trades {
				coinbasePrice = trade.Price
			}
		case tick := <-spot:
			for _, trade := range tick.Trades {
				spotPrice = trade.Price
				if futuresPrice.IsPositive() {
					del := spotPrice.Sub(futuresPrice).Abs().Div(spotPrice).BPS()
					if del.Cmp(decimal.FromInt(100)) > 0 {
						log.Printf("OUTLIER: spot=%s futures=%s del=%s", spotPrice, futuresPrice, del)
					}
					bes = min(bes, del)
					wor = max(wor, del)
					avg += (del.Float64() - avg) / cnt
					cnt++
					fmt.Printf("\r\033[K%s (avg %.3f, best %s, worst %s, n=%.0f, spot=%s, futures=%s, coinbase=%s)", del, avg, bes, wor, cnt, spotPrice, futuresPrice, coinbasePrice)
				}
			}
		case tick := <-futures:
			for _, trade := range tick.Trades {
				futuresPrice = trade.Price
				if spotPrice.IsPositive() {
					del := spotPrice.Sub(futuresPrice).Abs().Div(spotPrice).BPS()
					if del.Cmp(decimal.FromInt(100)) > 0 {
						log.Printf("OUTLIER: spot=%s futures=%s del=%s", spotPrice, futuresPrice, del)
					}
					bes = min(bes, del)
					wor = max(wor, del)
					avg += (del.Float64() - avg) / cnt
					cnt++
					fmt.Printf("\r\033[K%s (avg %.3f, best %s, worst %s, n=%.0f, spot=%s, futures=%s, coinbase=%s)", del, avg, bes, wor, cnt, spotPrice, futuresPrice, coinbasePrice)
				}
			}
		}
	}
}
