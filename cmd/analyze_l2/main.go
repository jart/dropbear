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
	flagDataset = flag.String("dataset", "bone", "dataset name")
	flagSymbol  = flag.String("symbol", "BTC", "symbol to analyze (e.g. BTC, ETH, SOL)")
)

// PriceEvent represents a price observation at a point in time
type PriceEvent struct {
	Time  clocky.Time
	Price decimal.Decimal
}

func loadTicks(path string) ([]ds.Tick, error) {
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
	var ticks []ds.Tick
	for {
		var tick ds.Tick
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
func extractTrades(ticks []ds.Tick) []PriceEvent {
	var events []PriceEvent
	for _, tick := range ticks {
		for _, trade := range tick.Trades {
			events = append(events, PriceEvent{
				Time:  trade.Time,
				Price: trade.Price,
			})
		}
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})
	return events
}

// Extract mid prices from L2 ticks using ds.Book for O(log n) updates
// Uses $1000 depth to ignore thin liquidity at NBBO
func extractMids(ticks []ds.Tick) []PriceEvent {
	var events []PriceEvent
	book := ds.NewBook()
	depth := decimal.Parse("1000") // $1000 liquidity depth

	for _, tick := range ticks {
		if tick.Snap {
			book.Clear()
		}
		for _, bid := range tick.Bids {
			book.UpdateBid(bid.Price, bid.Size)
		}
		for _, ask := range tick.Asks {
			book.UpdateAsk(ask.Price, ask.Size)
		}
		if len(tick.Bids) == 0 && len(tick.Asks) == 0 {
			continue
		}
		bid := book.PickBidByValue(depth)
		ask := book.PickAskByValue(depth)
		if bid.IsZero() || ask.IsZero() {
			continue
		}
		mid := bid.Add(ask).DivInt(2)
		events = append(events, PriceEvent{Time: tick.Time, Price: mid})
	}
	return events
}

// Find the price at or just before the given time
func priceAt(events []PriceEvent, t clocky.Time) (decimal.Decimal, bool) {
	idx := sort.Search(len(events), func(i int) bool {
		return events[i].Time.After(t)
	})
	if idx == 0 {
		return decimal.Zero, false
	}
	return events[idx-1].Price, true
}

// Compute correlation
func correlation(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) < 10 {
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

// analyzeLeadTime measures how predictor returns correlate with target returns at various lags
// Samples at predictor events, not target events
func analyzeLeadTime(name string, predictor, target []PriceEvent, lags []int) {
	if len(predictor) < 100 || len(target) < 100 {
		fmt.Printf("%s: insufficient data\n", name)
		return
	}

	fmt.Printf("\n--- %s ---\n", name)
	fmt.Printf("Lag(ms)  Corr     Samples\n")

	bestLag := 0
	bestCorr := -999.0

	for _, lagMs := range lags {
		lag := clocky.Duration(lagMs) * clocky.Millisecond

		var predReturns, targetReturns []float64

		// Sample at predictor events
		for i := 1; i < len(predictor); i++ {
			predTime := predictor[i].Time
			prevPredTime := predictor[i-1].Time

			// Skip if events too close (avoid noise) or too far (different regime)
			dt := predTime.Sub(prevPredTime)
			if dt < clocky.Millisecond || dt > 10*clocky.Second {
				continue
			}

			// Predictor return
			predRet := predictor[i].Price.Sub(predictor[i-1].Price).Div(predictor[i-1].Price).Float64()

			// Target return at (predictor_time + lag)
			targetPrice1, ok1 := priceAt(target, prevPredTime.Add(lag))
			targetPrice2, ok2 := priceAt(target, predTime.Add(lag))
			if !ok1 || !ok2 || targetPrice1.IsZero() || targetPrice2.IsZero() {
				continue
			}
			targetRet := targetPrice2.Sub(targetPrice1).Div(targetPrice1).Float64()

			if !math.IsNaN(predRet) && !math.IsNaN(targetRet) &&
				!math.IsInf(predRet, 0) && !math.IsInf(targetRet, 0) &&
				math.Abs(predRet) < 0.01 && math.Abs(targetRet) < 0.01 { // filter outliers
				predReturns = append(predReturns, predRet)
				targetReturns = append(targetReturns, targetRet)
			}
		}

		corr := correlation(predReturns, targetReturns)
		fmt.Printf("%7d  %9.4f  %9d\n", lagMs, corr, len(predReturns))

		if corr > bestCorr {
			bestCorr = corr
			bestLag = lagMs
		}
	}

	fmt.Printf("Peak: %dms @ %.4f\n", bestLag, bestCorr)
}

// analyzeDirectionLead measures: when predictor moves, how often does target follow in same direction?
func analyzeDirectionLead(name string, predictor, target []PriceEvent, lags []int, threshold decimal.Decimal) {
	if len(predictor) < 100 || len(target) < 100 {
		return
	}

	fmt.Printf("\n--- %s Direction (>%sbps move) ---\n", name, threshold.BPS().Format(1))
	fmt.Printf("Lag(ms)  Same%%   Samples\n")

	for _, lagMs := range lags {
		lag := clocky.Duration(lagMs) * clocky.Millisecond

		var same, total int

		for i := 1; i < len(predictor); i++ {
			predTime := predictor[i].Time
			prevPredTime := predictor[i-1].Time

			predMove := predictor[i].Price.Sub(predictor[i-1].Price)
			predRet := predMove.Div(predictor[i-1].Price).Abs()
			if predRet.Cmp(threshold) < 0 {
				continue // skip small moves
			}

			targetPrice1, ok1 := priceAt(target, prevPredTime.Add(lag))
			targetPrice2, ok2 := priceAt(target, predTime.Add(lag))
			if !ok1 || !ok2 || targetPrice1.IsZero() || targetPrice2.IsZero() {
				continue
			}
			targetMove := targetPrice2.Sub(targetPrice1)

			total++
			if (predMove.IsPositive() && targetMove.IsPositive()) ||
				(predMove.IsNegative() && targetMove.IsNegative()) {
				same++
			}
		}

		if total > 0 {
			pct := float64(same) * 100 / float64(total)
			fmt.Printf("%7d  %5.1f%%  %8d\n", lagMs, pct, total)
		}
	}
}

func main() {
	flag.Parse()

	homeDir, _ := os.UserHomeDir()
	dataPath := filepath.Join(homeDir, "marketdata", *flagDataset)
	symbol := strings.ToUpper(*flagSymbol)

	coinbaseSymbol := symbol + "-USD"
	coinbasePath := filepath.Join(dataPath, "coinbase", coinbaseSymbol)

	fmt.Printf("L2 Lead Analysis: %s (dataset: %s)\n", symbol, *flagDataset)

	// Load Coinbase
	coinbaseTicks, err := loadTicks(coinbasePath)
	if err != nil {
		log.Fatalf("Could not load Coinbase %s: %v", coinbaseSymbol, err)
	}
	coinbaseTrades := extractTrades(coinbaseTicks)
	coinbaseMids := extractMids(coinbaseTicks)
	fmt.Printf("Coinbase: %d trades, %d L2 events\n", len(coinbaseTrades), len(coinbaseMids))

	// Fine-grained lags for HFT
	lags := []int{-200, -175, -150, -125, -100, -75, -50, -25, 0, 25, 50, 75, 100, 125, 150, 175, 200}

	// Binance sources
	sources := []struct {
		Name   string
		Dir    string
		Symbol string
	}{
		{"FDUSD", "binance", symbol + "FDUSD"},
		{"USDT", "binance", symbol + "USDT"},
		{"USDC", "binance", symbol + "USDC"},
		{"USDT-Perp", "binanceusd", symbol + "USDT"},
		{"USDC-Perp", "binanceusd", symbol + "USDC"},
	}

	for _, src := range sources {
		path := filepath.Join(dataPath, src.Dir, src.Symbol)
		ticks, err := loadTicks(path)
		if err != nil {
			continue
		}

		trades := extractTrades(ticks)
		mids := extractMids(ticks)
		fmt.Printf("\n========================================\n")
		fmt.Printf("Binance %s: %d trades, %d L2\n", src.Name, len(trades), len(mids))
		fmt.Printf("========================================\n")

		if len(trades) > 0 {
			analyzeLeadTime("Trades->CB Trades", trades, coinbaseTrades, lags)
			analyzeLeadTime("Trades->CB Mid", trades, coinbaseMids, lags)
		}
		if len(mids) > 0 {
			analyzeLeadTime("L2 Mid->CB Trades", mids, coinbaseTrades, lags)
			analyzeLeadTime("L2 Mid->CB Mid", mids, coinbaseMids, lags)
		}

		// Direction analysis with 1bps threshold
		if len(trades) > 0 {
			analyzeDirectionLead("Trades->CB", trades, coinbaseTrades, lags, decimal.FromBPS(1))
		}
		if len(mids) > 0 {
			analyzeDirectionLead("L2->CB", mids, coinbaseTrades, lags, decimal.FromBPS(1))
		}
	}

	fmt.Println("\n========================================")
	fmt.Println("INTERPRETATION")
	fmt.Println("========================================")
	fmt.Println("Positive lag = predictor LEADS (we see predictor move, then target follows)")
	fmt.Println("Negative lag = predictor LAGS (target moved first)")
	fmt.Println("Peak at 0ms = simultaneous (probably same underlying cause)")
	fmt.Println("Higher correlation at lower lag = faster signal")
}
