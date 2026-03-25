package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"dropbear/broker/databento"
)

func main() {
	flag.Parse()
	key := databento.MustLoadDefaultKey()
	client := databento.NewHistoricalClient(key)
	datasets, err := client.ListDatasets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	for _, dataset := range datasets {
		schemas, err := client.ListSchemas(dataset)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dataset %s: %v\n", dataset, err)
			continue
		}
		fmt.Printf("%-16s %s\n", dataset, joinSchemas(schemas, ", "))
	}
}

func joinSchemas(schemas []databento.Schema, sep string) string {
	ss := make([]string, len(schemas))
	for i, s := range schemas {
		ss[i] = s.String()
	}
	return strings.Join(ss, sep)
}
