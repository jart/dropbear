// Generates price charts from market data recordings.
package main

import (
	"dropbear/clocky"
	"dropbear/ds"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
)

var (
	flagSymbol = flag.String("symbol", "BTC", "symbol to chart")
)

func main() {
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatalf("usage: chart <dataset-name>")
	}
	dataset := flag.Arg(0)

	// Open coinbase market data
	home := os.Getenv("HOME")
	path := fmt.Sprintf("%s/coindata/%s/coinbase/%s-USD", home, dataset, *flagSymbol)
	reader, err := ds.OpenTickReader(path)
	if err != nil {
		log.Fatalf("failed to open %s: %v", path, err)
	}
	defer reader.Close()

	// Collect price samples
	var prices []pricePoint
	var lastPrice float64
	sampleInterval := clocky.Minute

	for {
		tick := reader.Next()
		if tick.IsZero() {
			break
		}

		// Get price from trades
		for i := 0; i < tick.TradeCount(); i++ {
			lastPrice = tick.Trade(i).Price.Float64()
		}

		if lastPrice == 0 {
			continue
		}

		// Sample at intervals
		if len(prices) == 0 || tick.Time().Sub(prices[len(prices)-1].time) >= sampleInterval {
			prices = append(prices, pricePoint{
				time:  tick.Time(),
				price: lastPrice,
			})
		}
	}

	if len(prices) == 0 {
		log.Fatalf("no price data found")
	}

	// Calculate stats
	minPrice, maxPrice := prices[0].price, prices[0].price
	for _, p := range prices {
		if p.price < minPrice {
			minPrice = p.price
		}
		if p.price > maxPrice {
			maxPrice = p.price
		}
	}

	startTime := prices[0].time
	endTime := prices[len(prices)-1].time
	duration := endTime.Sub(startTime)
	change := (prices[len(prices)-1].price - prices[0].price) / prices[0].price * 100

	fmt.Printf("# %s %s-USD\n", dataset, *flagSymbol)
	fmt.Printf("# %s to %s (%s)\n", startTime, endTime, duration)
	fmt.Printf("# %.2f to %.2f (%.2f%%)\n", prices[0].price, prices[len(prices)-1].price, change)
	fmt.Printf("# range: %.2f - %.2f\n", minPrice, maxPrice)
	fmt.Println()

	// Write data to temp file for gnuplot
	tmpfile, err := os.CreateTemp("", "chart-*.dat")
	if err != nil {
		log.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	for i, p := range prices {
		fmt.Fprintf(tmpfile, "%d %.2f\n", i, p.price)
	}
	tmpfile.Close()

	// Generate gnuplot script
	script := fmt.Sprintf(`
set terminal dumb 120 30
set title "%s %s-USD (%.2f%% change)"
set xlabel "Time (minutes)"
set ylabel "Price ($)"
set nokey
plot "%s" using 1:2 with lines
`, dataset, *flagSymbol, change, tmpfile.Name())

	cmd := exec.Command("gnuplot", "-e", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Fallback to simple ASCII if gnuplot not available
		printASCII(prices, minPrice, maxPrice)
	}
}

type pricePoint struct {
	time  clocky.Time
	price float64
}

func printASCII(prices []pricePoint, minPrice, maxPrice float64) {
	height := 20
	width := 80
	if len(prices) < width {
		width = len(prices)
	}

	// Resample to fit width
	step := len(prices) / width
	if step == 0 {
		step = 1
	}

	var sampled []float64
	for i := 0; i < len(prices); i += step {
		sampled = append(sampled, prices[i].price)
		if len(sampled) >= width {
			break
		}
	}

	priceRange := maxPrice - minPrice
	if priceRange == 0 {
		priceRange = 1
	}

	// Draw chart
	for row := height - 1; row >= 0; row-- {
		threshold := minPrice + priceRange*float64(row)/float64(height-1)
		if row == height-1 {
			fmt.Printf("%8.0f |", maxPrice)
		} else if row == 0 {
			fmt.Printf("%8.0f |", minPrice)
		} else if row == height/2 {
			fmt.Printf("%8.0f |", (minPrice+maxPrice)/2)
		} else {
			fmt.Printf("         |")
		}
		for _, p := range sampled {
			if p >= threshold {
				fmt.Print("*")
			} else {
				fmt.Print(" ")
			}
		}
		fmt.Println()
	}
	fmt.Printf("         +%s\n", string(make([]byte, len(sampled))))
}
