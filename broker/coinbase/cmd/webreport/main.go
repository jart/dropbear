// webreport.coinbase generates static HTML reports for Coinbase trading performance.
//
// Usage:
//
//	go run ./cmd/webreport.coinbase \
//	  -db ~/.zec.sqlite3 \
//	  -coinbase-key ~/.zec.key \
//	  -output /var/www/justinestreet.capital/eth/ \
//	  -asset ETH \
//	  -genesis 2025-12-28 \
//	  -quantum 1h
package main

import (
	"dropbear/broker/coinbase"
	"dropbear/clocky"
	_ "dropbear/db" // registers -db flag
	"dropbear/loggy"
	"flag"
	"os"
)

var (
	flagOutput    = flag.String("output", ".", "output directory for static files")
	flagAsset     = flag.String("asset", "ETH", "asset being traded (e.g., ETH, BTC)")
	flagGenesis   = clocky.TimeFlag("genesis", "", "start date for analysis (e.g., 2025-12-28)")
	flagQuantum   = clocky.DurationFlag("quantum", "1h", "sampling interval for portfolio value")
	flagAlgorithm = flag.String("algorithm", "market taking algorithm", "algorithm name for report title")
)

func main() {
	loggy.Init()
	flag.Parse()

	if flagGenesis.IsZero() {
		loggy.Fatalf("-genesis flag is required")
	}

	// create output directory if needed
	if err := os.MkdirAll(*flagOutput, 0755); err != nil {
		loggy.Fatalf("creating output directory: %v", err)
	}

	// create coinbase client (uses -coinbase-key flag)
	client := coinbase.NewClient()

	// generate the report
	if err := generateReport(client, *flagAsset, *flagAlgorithm, *flagGenesis, *flagQuantum, *flagOutput); err != nil {
		loggy.Fatalf("generating report: %v", err)
	}
}
