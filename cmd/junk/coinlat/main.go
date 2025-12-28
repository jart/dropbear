// coinlat measures Coinbase API latency by pinging and placing an order.
package main

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/coinbase"
	"dropbear/loggy"
	"flag"
	"io"
	"log"
	"net/http"
	"time"
)

var (
	flagQuantity = decimal.Flag("qty", "0.000023", "BTC quantity to buy")
	flagPings    = flag.Int("pings", 3, "number of pings before order")
)

func main() {
	flag.Parse()
	loggy.Init()

	client := coinbase.NewClient()

	// ping coinbase time API
	log.Printf("[ping] warming up connection...")
	for i := range *flagPings {
		start := time.Now()
		req, err := http.NewRequest("GET", "https://api.coinbase.com/v2/time", nil)
		if err != nil {
			loggy.Fatalf("creating request: %v", err)
		}
		resp, err := ds.FastHTTPClient.Do(req)
		if err != nil {
			loggy.Fatalf("ping failed: %v", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		elapsed := time.Since(start)
		log.Printf("[ping] %d: %s (status %d)", i+1, elapsed, resp.StatusCode)
	}

	// place market buy order for BTC
	log.Printf("[order] placing market buy for %s BTC...", *flagQuantity)
	orderStart := time.Now()
	orderID, err := client.MarketOrder("BTC-USD", ds.SideBuy, *flagQuantity, "", ds.CostBasisMethodFIFO)
	orderSent := time.Now()
	if err != nil {
		loggy.Fatalf("order failed: %v", err)
	}
	log.Printf("[order] submitted in %s, order_id=%s", orderSent.Sub(orderStart), orderID)

	// fetch order details to confirm fill
	order, err := client.GetOrder(orderID)
	orderAck := time.Now()
	if err != nil {
		loggy.Fatalf("get order failed: %v", err)
	}

	log.Printf("[fill] status=%s filled=%s price=%s fees=%s",
		order.Status, order.FilledSize, order.AverageFilledPrice, order.TotalFees)
	log.Printf("[perf] order submitted in %s, confirmed in %s, round-trip %s",
		orderSent.Sub(orderStart), orderAck.Sub(orderSent), orderAck.Sub(orderStart))
}
