// cmd/analyze profiles spread deviation distribution across datasets.
// Usage: go run ./cmd/analyze -dataset chaos -symbol BTC
package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	"github.com/klauspost/compress/zstd"
)

var (
	flagDataset = flag.String("dataset", "chaos", "dataset name")
	flagSymbol  = flag.String("symbol", "BTC", "coinbase product to analyze")
	flagSize    = decimal.Flag("size", "200", "order size in usd for depth calculation")
	flagSamples = flag.Int("samples", 7000, "number of samples for baseline ema")
	flagBuckets = flag.Int("buckets", 20, "number of histogram buckets")
)

// Stats holds distribution statistics
type Stats struct {
	Count    int
	Mean     float64
	StdDev   float64
	Skewness float64
	Kurtosis float64
	Min      float64
	Max      float64
	Median   float64
	P5       float64
	P95      float64
}

func main() {
	flag.Parse()

	home := os.Getenv("HOME")

	// Determine binance symbol (no dash, e.g., BTCFDUSD)
	binanceSymbol := *flagSymbol + "FDUSD"
	if *flagSymbol == "ZEC" {
		binanceSymbol = *flagSymbol + "USDT"
	}

	// Open market data files
	coinbasePath := home + "/marketdata/" + *flagDataset + "/coinbase/" + *flagSymbol + "-USD"
	binancePath := home + "/marketdata/" + *flagDataset + "/binance/" + binanceSymbol

	coinbaseReader := openMarketData(coinbasePath)
	binanceReader := openMarketData(binancePath)
	defer coinbaseReader.Close()
	defer binanceReader.Close()

	// Initialize state
	spreadEMA := indicators.NewWWMA(*flagSamples)
	coinbaseBook := ds.NewBook()
	var binancePrice decimal.Decimal
	var deviations []float64
	warmedUp := false

	// Read and merge ticks by time
	var coinbaseTick, binanceTick ds.Tick
	coinbaseErr := coinbaseReader.Read(&coinbaseTick)
	binanceErr := binanceReader.Read(&binanceTick)

	for coinbaseErr == nil || binanceErr == nil {
		// Process the earlier tick
		processCoinbase := false
		processBinance := false

		if coinbaseErr != nil {
			processBinance = true
		} else if binanceErr != nil {
			processCoinbase = true
		} else if coinbaseTick.Time.Before(binanceTick.Time) {
			processCoinbase = true
		} else {
			processBinance = true
		}

		if processCoinbase {
			// Update order book
			coinbaseBook.Lock.Lock()
			if coinbaseTick.Snap {
				coinbaseBook.Clear()
			}
			for _, bid := range coinbaseTick.Bids {
				coinbaseBook.UpdateBid(bid.Price, bid.Size)
			}
			for _, ask := range coinbaseTick.Asks {
				coinbaseBook.UpdateAsk(ask.Price, ask.Size)
			}
			coinbaseBook.Lock.Unlock()

			// Record deviation if we have binance price
			if binancePrice.IsPositive() {
				dev := recordDeviation(coinbaseBook, binancePrice, spreadEMA, *flagSize, &warmedUp)
				if dev != nil {
					deviations = append(deviations, *dev)
				}
			}

			coinbaseErr = coinbaseReader.Read(&coinbaseTick)
		}

		if processBinance {
			// Update binance price from trades
			for _, trade := range binanceTick.Trades {
				if trade.Price.IsPositive() {
					binancePrice = trade.Price
				}
			}

			// Record deviation if we have order book data
			bestBid, _ := coinbaseBook.BestBidAsk()
			if bestBid.IsPositive() {
				dev := recordDeviation(coinbaseBook, binancePrice, spreadEMA, *flagSize, &warmedUp)
				if dev != nil {
					deviations = append(deviations, *dev)
				}
			}

			binanceErr = binanceReader.Read(&binanceTick)
		}
	}

	// Print results
	printResults(*flagSymbol, *flagDataset, deviations, *flagBuckets)
}

type marketDataReader struct {
	file   *os.File
	reader *zstd.Decoder
}

func openMarketData(path string) *marketDataReader {
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open %s: %v\n", path, err)
		os.Exit(1)
	}
	reader, err := zstd.NewReader(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create zstd reader: %v\n", err)
		os.Exit(1)
	}
	return &marketDataReader{file: file, reader: reader}
}

func (m *marketDataReader) Read(tick *ds.Tick) error {
	return tick.Deserialize(m.reader)
}

func (m *marketDataReader) Close() {
	m.reader.Close()
	m.file.Close()
}

func recordDeviation(book *ds.Book, binancePrice decimal.Decimal, ema *indicators.WWMA, size decimal.Decimal, warmedUp *bool) *float64 {
	if binancePrice.IsZero() {
		return nil
	}

	// Compute spread
	depth := size.Div(binancePrice)
	bid := book.PickBid(depth)
	ask := book.PickAsk(depth)
	if bid.IsZero() || ask.IsZero() {
		return nil
	}

	coinbasePrice := bid.Add(ask).DivInt(2)
	spread := coinbasePrice.Sub(binancePrice).Div(binancePrice)

	ema.Add(spread)
	if !ema.IsReady() {
		return nil
	}

	if !*warmedUp {
		*warmedUp = true
	}

	baseline := ema.Value
	deviation := spread.Sub(baseline)
	devBPS := deviation.BPS().Float64()
	return &devBPS
}

func ignore(err error) {
	if err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
}

// Suppress unused variable warning
var _ = clocky.Now

func printResults(symbol, dataset string, deviations []float64, buckets int) {
	if len(deviations) == 0 {
		fmt.Println("No deviations recorded")
		return
	}

	stats := computeStats(deviations)

	fmt.Println()
	fmt.Printf("# Spread Deviation Analysis: %s/%s\n", dataset, symbol)
	fmt.Printf("# Samples: %d\n", stats.Count)
	fmt.Println()

	// Summary statistics
	fmt.Println("## Statistics (basis points)")
	fmt.Printf("mean     = %+.4f\n", stats.Mean)
	fmt.Printf("stddev   = %.4f\n", stats.StdDev)
	fmt.Printf("skewness = %+.4f\n", stats.Skewness)
	fmt.Printf("kurtosis = %.4f\n", stats.Kurtosis)
	fmt.Printf("min      = %+.4f\n", stats.Min)
	fmt.Printf("max      = %+.4f\n", stats.Max)
	fmt.Printf("median   = %+.4f\n", stats.Median)
	fmt.Printf("p5       = %+.4f\n", stats.P5)
	fmt.Printf("p95      = %+.4f\n", stats.P95)
	fmt.Println()

	// Threshold analysis
	fmt.Println("## Threshold Crossings")
	thresholds := []float64{1, 2, 3, 4, 5, 7, 10}
	for _, t := range thresholds {
		buyCount := 0
		sellCount := 0
		for _, d := range deviations {
			if d < -t {
				buyCount++
			}
			if d > t {
				sellCount++
			}
		}
		buyPct := float64(buyCount) / float64(len(deviations)) * 100
		sellPct := float64(sellCount) / float64(len(deviations)) * 100
		fmt.Printf("threshold=%2.0fbps: buy_signals=%5.2f%% sell_signals=%5.2f%%\n", t, buyPct, sellPct)
	}
	fmt.Println()

	// Histogram
	fmt.Println("## Histogram")
	printHistogram(deviations, buckets)
}

func computeStats(data []float64) Stats {
	n := len(data)
	if n == 0 {
		return Stats{}
	}

	// Sort for percentiles
	sorted := make([]float64, n)
	copy(sorted, data)
	sort.Float64s(sorted)

	// Mean
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	mean := sum / float64(n)

	// Variance, skewness, kurtosis
	variance := 0.0
	skewSum := 0.0
	kurtSum := 0.0
	for _, v := range data {
		diff := v - mean
		variance += diff * diff
		skewSum += diff * diff * diff
		kurtSum += diff * diff * diff * diff
	}
	variance /= float64(n)
	stddev := math.Sqrt(variance)

	skewness := 0.0
	kurtosis := 0.0
	if stddev > 0 {
		skewness = (skewSum / float64(n)) / (stddev * stddev * stddev)
		kurtosis = (kurtSum / float64(n)) / (variance * variance)
	}

	return Stats{
		Count:    n,
		Mean:     mean,
		StdDev:   stddev,
		Skewness: skewness,
		Kurtosis: kurtosis,
		Min:      sorted[0],
		Max:      sorted[n-1],
		Median:   sorted[n/2],
		P5:       sorted[n*5/100],
		P95:      sorted[n*95/100],
	}
}

func printHistogram(data []float64, buckets int) {
	if len(data) == 0 {
		return
	}

	// Find range
	minVal := data[0]
	maxVal := data[0]
	for _, v := range data {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// Symmetric range around 0
	absMax := math.Max(math.Abs(minVal), math.Abs(maxVal))
	minVal = -absMax
	maxVal = absMax

	bucketSize := (maxVal - minVal) / float64(buckets)
	counts := make([]int, buckets)

	for _, v := range data {
		idx := int((v - minVal) / bucketSize)
		if idx >= buckets {
			idx = buckets - 1
		}
		if idx < 0 {
			idx = 0
		}
		counts[idx]++
	}

	// Find max count for scaling
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}

	// Print histogram
	barWidth := 50
	for i := 0; i < buckets; i++ {
		low := minVal + float64(i)*bucketSize
		high := low + bucketSize
		pct := float64(counts[i]) / float64(len(data)) * 100
		barLen := 0
		if maxCount > 0 {
			barLen = counts[i] * barWidth / maxCount
		}
		bar := ""
		for j := 0; j < barLen; j++ {
			bar += "█"
		}
		fmt.Printf("%+6.2f to %+6.2f | %5.2f%% %s\n", low, high, pct, bar)
	}
}
