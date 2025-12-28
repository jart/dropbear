package main

import (
	"dropbear/clocky"
	"dropbear/indicators"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/klauspost/compress/zstd"
)

var (
	flagLimit = flag.Int("n", 0, "limit number of candles to print (0 = all)")
	flagStart = clocky.TimeFlag("start", "", "only print candles at or after this time")
	flagEnd   = clocky.TimeFlag("end", "", "only print candles before this time")
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: dumpcandles [-n limit] <file>...\n")
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
		var candle indicators.Candle
		err = candle.Deserialize(r)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
		if *flagStart != 0 && candle.Start < *flagStart {
			continue
		}
		if *flagEnd != 0 && candle.Start >= *flagEnd {
			continue
		}
		fmt.Printf("%s  O=%s H=%s L=%s C=%s V=%s\n",
			candle.Start,
			candle.Open,
			candle.High,
			candle.Low,
			candle.Close,
			candle.Volume)
		count++
		if *flagLimit > 0 && count >= *flagLimit {
			break
		}
	}
	return nil
}
