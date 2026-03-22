package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"dropbear/broker/databento"
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: dbncount <file.dbn> ...\n")
		os.Exit(1)
	}
	count := 0
	for _, path := range flag.Args() {
		if n, err := Count(path); err != nil {
			log.Fatal(err)
		} else {
			count += n
		}
	}
	fmt.Printf("%d\n", count)
}

func Count(path string) (int, error) {
	r, err := databento.OpenFileReader(path)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	count := 0
	for {
		_, err := r.Read()
		if err != nil {
			if err == io.EOF {
				return count, nil
			}
			return 0, err
		}
		count++
	}
}
