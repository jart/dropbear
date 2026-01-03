package main

import (
	"dropbear/clocky"
	"dropbear/ds"
	"flag"
	"fmt"
	"log"
	"os"
)

var (
	flagLimit = flag.Int("n", 0, "limit number of ticks to print (0 = all)")
	flagStart = clocky.TimeFlag("start", "", "only print ticks at or after this time")
	flagEnd   = clocky.TimeFlag("end", "", "only print ticks before this time")
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: dumptick [-n limit] <file>...\n")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		if err := dump(path); err != nil {
			log.Fatalf("%s: %v", path, err)
		}
	}
}

func dump(path string) error {
	reader, err := ds.OpenTickReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	count := 0
	for {
		tick := reader.Next()
		if tick.IsZero() {
			break
		}
		tickTime := tick.Time()
		if *flagStart != 0 && tickTime < *flagStart {
			continue
		}
		if *flagEnd != 0 && tickTime >= *flagEnd {
			continue
		}
		fmt.Printf("tick %s\n", tickTime)
		if tick.Snap() {
			fmt.Printf("  snap\n")
		}
		for i := 0; i < tick.BidCount(); i++ {
			bid := tick.Bid(i)
			fmt.Printf("  bid %s @ %s\n", bid.Size, bid.Price)
		}
		for i := 0; i < tick.AskCount(); i++ {
			ask := tick.Ask(i)
			fmt.Printf("  ask %s @ %s\n", ask.Size, ask.Price)
		}
		for i := 0; i < tick.TradeCount(); i++ {
			trade := tick.Trade(i)
			fmt.Printf("  %s %s @ %s @ %s\n", trade.Side, trade.Quantity, trade.Price, trade.Time)
		}
		count++
		if *flagLimit > 0 && count >= *flagLimit {
			break
		}
	}
	return nil
}
