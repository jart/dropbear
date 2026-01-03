package main

import (
	"dropbear/clocky"
	"dropbear/indicators"
	"flag"
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"
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

const candleSize = 48

func dump(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	if size == 0 {
		return nil
	}

	data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return err
	}
	defer syscall.Munmap(data)

	count := 0
	for offset := 0; offset+candleSize <= len(data); offset += candleSize {
		candle := (*indicators.Candle)(unsafe.Pointer(&data[offset]))
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
