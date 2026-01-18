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
	flagLimit = flag.Int("n", 0, "limit number of bars to print (0 = all)")
	flagStart = clocky.TimeFlag("start", "", "only print bars at or after this time")
	flagEnd   = clocky.TimeFlag("end", "", "only print bars before this time")
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: dumpbars [-n limit] <file>...\n")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		if err := dump(path); err != nil {
			log.Fatalf("%s: %v", path, err)
		}
	}
}

func dump(path string) error {
	bars, err := ds.OpenBars(path)
	if err != nil {
		return err
	}
	defer bars.Close()

	count := 0
	for !bars.EOF() {
		bar := bars.Read()
		if *flagStart != 0 && bar.Timestamp < *flagStart {
			continue
		}
		if *flagEnd != 0 && bar.Timestamp >= *flagEnd {
			continue
		}
		fmt.Printf("%s  O=%s H=%s L=%s C=%s V=%s\n",
			bar.Timestamp,
			bar.Open,
			bar.High,
			bar.Low,
			bar.Close,
			bar.Volume)
		count++
		if *flagLimit > 0 && count >= *flagLimit {
			break
		}
	}
	return nil
}
