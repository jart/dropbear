package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"
)

var (
	flagDataset = flag.String("dataset", "heart", "dataset name")
	flagSymbol  = flag.String("symbol", "BTC", "symbol to analyze (e.g. BTC, ETH, SOL)")
)

// Source defines a market data source to analyze
type Source struct {
	Name   string // Display name
	Dir    string // Directory under dataset (binance, binanceusd, coinbase)
	Symbol string // Symbol name (computed from base symbol)
}

// PriceEvent represents a price observation at a point in time
type PriceEvent struct {
	Time  clocky.Time
	Price decimal.Decimal
	Kind  string // "trade" or "mid"
}

func loadTicks(path string) ([]ds.TickBuilder, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r, err := zstd.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var ticks []ds.TickBuilder
	for {
		var tick ds.TickBuilder
		err = tick.Deserialize(r)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, err
		}
		ticks = append(ticks, tick)
	}
	return ticks, nil
}

// Extract trade prices from ticks
func extractTrades(ticks []ds.TickBuilder) []PriceEvent {
	var events []PriceEvent
	for _, tick := range ticks {
		for _, trade := range tick.Trades {
			events = append(events, PriceEvent{
				Time:  trade.Time,
				Price: trade.Price,
				Kind:  "trade",
			})
		}
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})
	return events
}

// Extract mid prices from ticks (when book updates happen)
func extractMids(ticks []ds.TickBuilder) []PriceEvent {
	var events []PriceEvent
	for _, tick := range ticks {
		if len(tick.Bids) == 0 || len(tick.Asks) == 0 {
			continue
		}
		// Find best bid and ask
		var bestBid, bestAsk decimal.Decimal
		for _, bid := range tick.Bids {
			if bestBid.IsZero() || bid.Price.Cmp(bestBid) > 0 {
				bestBid = bid.Price
			}
		}
		for _, ask := range tick.Asks {
			if bestAsk.IsZero() || ask.Price.Cmp(bestAsk) < 0 {
				bestAsk = ask.Price
			}
		}
		if !bestBid.IsZero() && !bestAsk.IsZero() {
			mid := bestBid.Add(bestAsk).DivInt(2)
			events = append(events, PriceEvent{
				Time:  tick.Time,
				Price: mid,
				Kind:  "mid",
			})
		}
	}
	return events
}

// Find the price at or just before the given time
func priceAt(events []PriceEvent, t clocky.Time) (decimal.Decimal, bool) {
	// Binary search for the last event <= t
	idx := sort.Search(len(events), func(i int) bool {
		return events[i].Time.After(t)
	})
	if idx == 0 {
		var zero decimal.Decimal
		return zero, false
	}
	return events[idx-1].Price, true
}

// Compute correlation between two price series
func computeCorrelation(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) == 0 {
		return 0
	}
	n := float64(len(xs))
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumX2 += xs[i] * xs[i]
		sumY2 += ys[i] * ys[i]
	}
	num := n*sumXY - sumX*sumY
	den := math.Sqrt((n*sumX2 - sumX*sumX) * (n*sumY2 - sumY*sumY))
	if den == 0 {
		return 0
	}
	return num / den
}

// Compute how well a predictor leads target at various lags
func analyzeLead(predictor, target []PriceEvent, name string) {
	if len(predictor) == 0 || len(target) == 0 {
		fmt.Printf("%s: no data\n", name)
		return
	}

	fmt.Printf("\n=== %s ===\n", name)
	fmt.Printf("Predictor events: %d, Target events: %d\n", len(predictor), len(target))
	fmt.Printf("Predictor range: %s to %s\n",
		predictor[0].Time,
		predictor[len(predictor)-1].Time)
	fmt.Printf("Target range: %s to %s\n",
		target[0].Time,
		target[len(target)-1].Time)

	// Test various lead times
	lags := []int{-1000, -800, -600, -500, -400, -300, -200, -100, -50, 0, 50, 100, 200, 300, 400, 500, 600, 800, 1000}

	fmt.Printf("\nLag (ms)  Correlation  Samples\n")
	fmt.Printf("--------  -----------  -------\n")

	bestLag := 0
	bestCorr := -999.0

	for _, lagMs := range lags {
		lag := clocky.Duration(lagMs) * clocky.Millisecond

		var predictorReturns, targetReturns []float64

		// For each target trade, find predictor price at (target_time - lag) and target_time
		// Then compute returns
		for i := 1; i < len(target); i++ {
			targetTime := target[i].Time
			prevTargetTime := target[i-1].Time

			// Get predictor price at shifted times
			predPrice1, ok1 := priceAt(predictor, prevTargetTime.Add(-lag))
			predPrice2, ok2 := priceAt(predictor, targetTime.Add(-lag))
			if !ok1 || !ok2 || predPrice1.IsZero() || predPrice2.IsZero() {
				continue
			}

			// Compute returns
			predReturn := predPrice2.Sub(predPrice1).Div(predPrice1).Float64()
			targetReturn := target[i].Price.Sub(target[i-1].Price).Div(target[i-1].Price).Float64()

			if !math.IsNaN(predReturn) && !math.IsNaN(targetReturn) &&
				!math.IsInf(predReturn, 0) && !math.IsInf(targetReturn, 0) {
				predictorReturns = append(predictorReturns, predReturn)
				targetReturns = append(targetReturns, targetReturn)
			}
		}

		corr := computeCorrelation(predictorReturns, targetReturns)
		fmt.Printf("%8d  %11.4f  %7d\n", lagMs, corr, len(predictorReturns))

		if corr > bestCorr {
			bestCorr = corr
			bestLag = lagMs
		}
	}

	fmt.Printf("\nBest lag: %dms with correlation %.4f\n", bestLag, bestCorr)
}

// Count how often predictor moves before target moves same direction
func analyzeLeadDirection(predictor, target []PriceEvent, name string) {
	if len(predictor) < 10 || len(target) < 10 {
		return
	}

	fmt.Printf("\n--- Direction Analysis: %s ---\n", name)

	// Sample every 100ms
	sampleInterval := 100 * clocky.Millisecond

	startTime := predictor[0].Time
	if target[0].Time.After(startTime) {
		startTime = target[0].Time
	}
	endTime := predictor[len(predictor)-1].Time
	if target[len(target)-1].Time.Before(endTime) {
		endTime = target[len(target)-1].Time
	}

	// Test various lead times
	testLags := []int{100, 200, 300, 400, 500, 600, 700, 800, 1000}

	fmt.Printf("\nLead (ms)  Correct%%  Samples  Predictor moved first\n")
	fmt.Printf("---------  --------  -------  --------------------\n")

	for _, leadMs := range testLags {
		lead := clocky.Duration(leadMs) * clocky.Millisecond

		var correctDir, total, predLeads int

		for t := startTime.Add(lead); t.Before(endTime); t = t.Add(sampleInterval) {
			// Get prices
			predNow, ok1 := priceAt(predictor, t.Add(-lead))
			predPrev, ok2 := priceAt(predictor, t.Add(-lead).Add(-sampleInterval))
			targetNow, ok3 := priceAt(target, t)
			targetPrev, ok4 := priceAt(target, t.Add(-sampleInterval))

			if !ok1 || !ok2 || !ok3 || !ok4 {
				continue
			}

			predMove := predNow.Sub(predPrev)
			targetMove := targetNow.Sub(targetPrev)

			if predMove.IsZero() || targetMove.IsZero() {
				continue
			}

			total++
			predUp := predMove.IsPositive()
			targetUp := targetMove.IsPositive()

			if predUp == targetUp {
				correctDir++
			}

			// Check if predictor moved before target
			if predMove.Abs().Cmp(targetMove.Abs()) > 0 {
				predLeads++
			}
		}

		if total > 0 {
			pct := float64(correctDir) * 100 / float64(total)
			leadPct := float64(predLeads) * 100 / float64(total)
			fmt.Printf("%9d  %7.1f%%  %7d  %18.1f%%\n", leadMs, pct, total, leadPct)
		}
	}
}

// buildSources returns the list of predictor sources for a given base symbol
func buildSources(symbol string) []Source {
	sym := strings.ToUpper(symbol)
	return []Source{
		{Name: "Binance " + sym + "USDT Perp", Dir: "binanceusd", Symbol: sym + "USDT"},
		{Name: "Binance " + sym + "USDC Perp", Dir: "binanceusd", Symbol: sym + "USDC"},
		{Name: "Binance " + sym + "FDUSD Spot", Dir: "binance", Symbol: sym + "FDUSD"},
		{Name: "Binance " + sym + "USDT Spot", Dir: "binance", Symbol: sym + "USDT"},
		{Name: "Binance " + sym + "USDC Spot", Dir: "binance", Symbol: sym + "USDC"},
	}
}

// SourceData holds loaded data for a source
type SourceData struct {
	Source Source
	Ticks  []ds.TickBuilder
	Trades []PriceEvent
	Mids   []PriceEvent
}

func main() {
	flag.Parse()

	homeDir, _ := os.UserHomeDir()
	dataPath := filepath.Join(homeDir, "marketdata", *flagDataset)
	symbol := strings.ToUpper(*flagSymbol)

	// Target is always Coinbase
	coinbaseSymbol := symbol + "-USD"
	coinbasePath := filepath.Join(dataPath, "coinbase", coinbaseSymbol)

	fmt.Printf("Analyzing %s (dataset: %s)\n", symbol, *flagDataset)
	fmt.Println("Loading data...")

	// Load Coinbase (target)
	coinbaseTicks, err := loadTicks(coinbasePath)
	if err != nil {
		log.Fatalf("Could not load Coinbase %s: %v", coinbaseSymbol, err)
	}
	coinbaseTrades := extractTrades(coinbaseTicks)
	coinbaseMids := extractMids(coinbaseTicks)
	fmt.Printf("  Coinbase %s: %d ticks, %d trades\n", coinbaseSymbol, len(coinbaseTicks), len(coinbaseTrades))

	// Load all predictor sources
	sources := buildSources(symbol)
	var loaded []SourceData
	for _, src := range sources {
		path := filepath.Join(dataPath, src.Dir, src.Symbol)
		ticks, err := loadTicks(path)
		if err != nil {
			continue // Skip sources that don't exist
		}
		trades := extractTrades(ticks)
		mids := extractMids(ticks)
		fmt.Printf("  %s: %d ticks, %d trades\n", src.Name, len(ticks), len(trades))
		loaded = append(loaded, SourceData{
			Source: src,
			Ticks:  ticks,
			Trades: trades,
			Mids:   mids,
		})
	}

	if len(loaded) == 0 {
		log.Fatal("No predictor sources found")
	}

	// Analyze trade-to-trade correlation
	fmt.Println("\n========================================")
	fmt.Println("TRADE PRICE CORRELATION ANALYSIS")
	fmt.Println("========================================")
	for _, src := range loaded {
		if len(src.Trades) > 0 {
			analyzeLead(src.Trades, coinbaseTrades, src.Source.Name+" -> Coinbase (trades)")
		}
	}

	// Analyze mid-to-mid correlation
	fmt.Println("\n========================================")
	fmt.Println("MID PRICE CORRELATION ANALYSIS")
	fmt.Println("========================================")
	for _, src := range loaded {
		if len(src.Mids) > 0 {
			analyzeLead(src.Mids, coinbaseMids, src.Source.Name+" -> Coinbase (mids)")
		}
	}

	// Direction analysis
	fmt.Println("\n========================================")
	fmt.Println("DIRECTION PREDICTION ANALYSIS")
	fmt.Println("========================================")
	for _, src := range loaded {
		if len(src.Trades) > 0 {
			analyzeLeadDirection(src.Trades, coinbaseTrades, src.Source.Name+" -> Coinbase")
		}
	}
}
