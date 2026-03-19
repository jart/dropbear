// dbndump prints the contents of DBN files in human-readable format.
//
// Usage:
//
//	go run ./broker/databento/cmd/dbndump ~/databento/SPXW/2026-03-13.definition.dbn
//	go run ./broker/databento/cmd/dbndump ~/databento/SPXW/2026-03-13.cmbp-1.dbn
//	go run ./broker/databento/cmd/dbndump -n 10 ~/databento/SPXW/2026-03-13.cmbp-1.dbn
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"dropbear/broker/databento"
)

var (
	nFlag = flag.Int("n", 0, "max number of records to print (0 = all)")
	mFlag = flag.Bool("m", false, "print metadata only")
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: dbndump [-n count] [-m] <file.dbn> ...\n")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		if err := dump(path); err != nil {
			log.Fatal(err)
		}
	}
}

func dump(path string) error {
	r, err := databento.OpenFile(path)
	if err != nil {
		return err
	}
	defer r.Close()
	fmt.Printf("%#v\n", &r.Metadata)
	if *mFlag {
		return nil
	}
	count := 0
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if gs, ok := rec.(fmt.GoStringer); ok {
			fmt.Printf("%s\n", gs.GoString())
		} else {
			fmt.Printf("%#v\n", rec)
		}
		count++
		if *nFlag > 0 && count >= *nFlag {
			return nil
		}
	}
}
