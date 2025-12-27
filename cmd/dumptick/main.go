package main

import (
	"dropbear/ds"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/klauspost/compress/zstd"
)

var (
	flagLimit = flag.Int("n", 0, "limit number of ticks to print (0 = all)")
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
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r, err := zstd.NewReader(f)
	if err != nil {
		return err
	}
	defer r.Close()
	count := 0
	for {
		var tick ds.Tick
		err = tick.Deserialize(r)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
		fmt.Printf("tick %s\n", tick.Time)
		if tick.Snap {
			fmt.Printf("  snap\n")
		}
		for _, bid := range tick.Bids {
			fmt.Printf("  bid %s @ %s\n", bid.Size, bid.Price)
		}
		for _, ask := range tick.Asks {
			fmt.Printf("  ask %s @ %s\n", ask.Size, ask.Price)
		}
		for _, trade := range tick.Trades {
			fmt.Printf("  %s %s @ %s @ %s\n", trade.Side, trade.Quantity, trade.Price, trade.Time)
		}
		count++
		if *flagLimit > 0 && count >= *flagLimit {
			break
		}
	}
	return nil
}
