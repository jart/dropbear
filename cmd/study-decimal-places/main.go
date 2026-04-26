// study2 measures how often quote imbalance exceeds a threshold
// across the full SIP feed, broken down by price range.
//
// usage: go test -run Study -timeout 0 -- ~/sip/sipdata-2026-01-07.sip
package main

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/decimal"
	"dropbear/netty"
	"flag"
	"fmt"
	"log"
	"os"
)

func init() {
	netty.SetOffline()
}

var (
	flagDayOnly = flag.Bool("dayonly", false, "restrict to day session only")
)

var gMaxPrecision int
var gMaxValue decimal.Decimal
var gMinValue decimal.Decimal

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: go test -run Study -timeout 0 -- <sipfile>...\n")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		study(path)
	}
	fmt.Printf("max price precision: %d\n", gMaxPrecision)
	fmt.Printf("max price value: %s\n", gMaxValue)
	fmt.Printf("min price value: %s\n", gMinValue)
}

func study(path string) {
	f, err := sip.OpenFile(path)
	if err != nil {
		log.Fatalf("%s: %v", path, err)
	}
	defer f.Close()
	for {
		msg := f.Read()
		if msg == nil {
			break
		}
		switch msg.Type {
		case sip.MessageTypeQuote:
			quote := msg.Quote()
			gMaxPrecision = max(gMaxPrecision, quote.BidPrice.Precision())
			gMaxPrecision = max(gMaxPrecision, quote.AskPrice.Precision())
			gMaxValue = max(gMaxValue, quote.BidPrice)
			gMaxValue = max(gMaxValue, quote.AskPrice)
			gMinValue = min(gMinValue, quote.BidPrice)
			gMinValue = min(gMinValue, quote.AskPrice)
		case sip.MessageTypeTrade:
			trade := msg.Trade()
			gMaxPrecision = max(gMaxPrecision, trade.Price.Precision())
			gMaxValue = max(gMaxValue, trade.Price)
			gMinValue = min(gMinValue, trade.Price)
		}
	}
}
