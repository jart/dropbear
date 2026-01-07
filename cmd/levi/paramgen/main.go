// Command paramgen finds optimal trail/mingap/lookback parameters for each stock.
//
// Uses coordinate descent with golden section search for efficient optimization
// over continuous parameter ranges. Scores by Sharpe ratio.
//
// Usage:
//
//	go run ./cmd/levi/paramgen -o cmd/levi/params.go

package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var symbolSplitter = regexp.MustCompile(`[ \t,]+`)

var (
	flagDataDir  = flag.String("data", os.ExpandEnv("$HOME/equitydata/minutes"), "directory containing bar data")
	flagOutput   = flag.String("o", "", "output file (empty for stdout)")
	flagSymbols  = flag.String("symbols", "", "comma-separated symbols to process (empty for all)")
	flagWorkers  = flag.Int("workers", runtime.NumCPU()*2, "number of parallel workers")
	flagCash     = decimal.Flag("cash", "100_000", "initial cash for simulation")
	flagMinPrice = decimal.Flag("minprice", "10", "minimum share price")
	flagVerbose  = flag.Bool("v", false, "verbose output")
)

// Parameter bounds for optimization
const (
	trailMin    = 0.001 // 0.1%
	trailMax    = 0.15  // 15%
	mingapMin   = 0.001 // 0.1%
	mingapMax   = 0.15  // 15%
	lookbackMin = 3     // 3 minutes
	lookbackMax = 300   // 5 hours (must fit within trading day since we reset daily)
)

// Golden ratio for golden section search
const phi = 1.6180339887498949

const (
	openTime  = 6_45_00
	closeTime = 12_45_00
)

// Result holds the optimal parameters for a symbol
type Result struct {
	Symbol   string
	Trail    decimal.Decimal
	MinGap   decimal.Decimal
	Lookback int
	Sharpe   float64
	Return   float64
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	// Find all symbols
	symbols, err := findSymbols()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Found %d symbols", len(symbols))

	// Process symbols in parallel
	results := make(chan Result, len(symbols))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *flagWorkers)
	var completed int64
	total := len(symbols)
	start := time.Now()

	// Progress logging goroutine
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n := atomic.LoadInt64(&completed)
				elapsed := time.Since(start)
				if n == 0 {
					log.Printf("Progress: 0/%d (0.0%%) - starting...", total)
					continue
				}
				rate := float64(n) / elapsed.Seconds()
				remaining := time.Duration(float64(int64(total)-n)/rate) * time.Second
				log.Printf("Progress: %d/%d (%.1f%%) - %.1f/sec - ETA %v",
					n, total, float64(n)/float64(total)*100, rate, remaining.Round(time.Second))
			case <-done:
				return
			}
		}
	}()

	for _, sym := range symbols {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := optimizeSymbol(symbol)
			atomic.AddInt64(&completed, 1)
			if err != nil {
				if *flagVerbose {
					log.Printf("%s: %v", symbol, err)
				}
				return
			}
			results <- result
			if *flagVerbose {
				log.Printf("%s: trail=%.1f%% mingap=%.2f%% lookback=%d sharpe=%.2f return=%.1f%%",
					symbol,
					result.Trail.MulInt(100).Float64(),
					result.MinGap.MulInt(100).Float64(),
					result.Lookback,
					result.Sharpe,
					result.Return*100)
			}
		}(sym)
	}

	go func() {
		wg.Wait()
		close(results)
		close(done)
	}()

	// Collect results
	var all []Result
	for r := range results {
		all = append(all, r)
	}

	// Sort by symbol
	sort.Slice(all, func(i, j int) bool {
		return all[i].Symbol < all[j].Symbol
	})

	log.Printf("Optimized %d symbols in %v", len(all), time.Since(start).Round(time.Second))

	// Output
	if err := writeOutput(all); err != nil {
		log.Fatal(err)
	}
}

func findSymbols() ([]string, error) {
	if *flagSymbols != "" {
		parts := symbolSplitter.Split(*flagSymbols, -1)
		var symbols []string
		for _, p := range parts {
			if p != "" {
				symbols = append(symbols, p)
			}
		}
		return symbols, nil
	}

	entries, err := os.ReadDir(*flagDataDir)
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, e := range entries {
		if !e.IsDir() {
			symbols = append(symbols, e.Name())
		}
	}
	return symbols, nil
}

func optimizeSymbol(symbol string) (Result, error) {
	// Load all bars for this symbol
	bars, err := loadBars(symbol)
	if err != nil {
		return Result{}, err
	}
	defer bars.Close()

	if bars.Count() < 1000 {
		return Result{}, fmt.Errorf("insufficient data: %d bars", bars.Count())
	}

	// Objective function
	eval := func(t, m float64, l int) float64 {
		sharpe, _ := simulate(bars, decimal.FromFloat64(t), decimal.FromFloat64(m), l)
		if math.IsInf(sharpe, -1) {
			return -1e9
		}
		return sharpe
	}

	// Phase 1: Coarse grid search to find a valid starting region
	coarseTrail := []float64{0.005, 0.01, 0.02, 0.03, 0.05, 0.08}
	coarseMingap := []float64{0.005, 0.01, 0.02, 0.03, 0.05}
	coarseLookback := []int{5, 10, 20, 50, 100, 200}

	var trail, mingap float64
	var lookback int
	bestSharpe := -1e9

	for _, t := range coarseTrail {
		for _, m := range coarseMingap {
			for _, l := range coarseLookback {
				if s := eval(t, m, l); s > bestSharpe {
					bestSharpe = s
					trail, mingap, lookback = t, m, l
				}
			}
		}
	}

	if bestSharpe <= -1e9 {
		return Result{}, fmt.Errorf("no valid parameter combination found")
	}

	if *flagVerbose {
		log.Printf("  coarse: trail=%.2f%% mingap=%.2f%% lookback=%d sharpe=%.2f",
			trail*100, mingap*100, lookback, bestSharpe)
	}

	// Phase 2: Refine with coordinate descent + golden section search
	// Search in expanding neighborhood around coarse solution
	bestTrail, bestMingap, bestLookback := trail, mingap, lookback
	for iter := 0; iter < 3; iter++ {
		// Define search bounds as neighborhood (expanding each iteration)
		factor := float64(iter + 1)
		trailLo := math.Max(trailMin, trail/factor/2)
		trailHi := math.Min(trailMax, trail*factor*2)
		mingapLo := math.Max(mingapMin, mingap/factor/2)
		mingapHi := math.Min(mingapMax, mingap*factor*2)
		lookbackLo := int(math.Max(float64(lookbackMin), float64(lookback)/factor/2))
		lookbackHi := int(math.Min(float64(lookbackMax), float64(lookback)*factor*2))

		// Optimize trail
		trail = goldenMax(trailLo, trailHi, 1e-4, func(t float64) float64 {
			return eval(t, mingap, lookback)
		})

		// Optimize mingap
		mingap = goldenMax(mingapLo, mingapHi, 1e-4, func(m float64) float64 {
			return eval(trail, m, lookback)
		})

		// Optimize lookback
		lookback = goldenMaxInt(lookbackLo, lookbackHi, func(l int) float64 {
			return eval(trail, mingap, l)
		})

		// Track best solution
		if s := eval(trail, mingap, lookback); s > bestSharpe {
			bestSharpe = s
			bestTrail, bestMingap, bestLookback = trail, mingap, lookback
		}

		if *flagVerbose {
			log.Printf("  iter %d: trail=%.3f%% mingap=%.3f%% lookback=%d sharpe=%.2f",
				iter, trail*100, mingap*100, lookback, eval(trail, mingap, lookback))
		}
	}
	trail, mingap, lookback = bestTrail, bestMingap, bestLookback

	// Get final score
	sharpe, ret := simulate(bars, decimal.FromFloat64(trail), decimal.FromFloat64(mingap), lookback)
	if math.IsInf(sharpe, -1) {
		return Result{}, fmt.Errorf("no valid results")
	}

	return Result{
		Symbol:   symbol,
		Trail:    decimal.FromFloat64(trail),
		MinGap:   decimal.FromFloat64(mingap),
		Lookback: lookback,
		Sharpe:   sharpe,
		Return:   ret,
	}, nil
}

// goldenMax finds the maximum of f on [a, b] using golden section search.
// Tolerance tol controls when to stop.
func goldenMax(a, b, tol float64, f func(float64) float64) float64 {
	c := b - (b-a)/phi
	d := a + (b-a)/phi
	for b-a > tol {
		if f(c) > f(d) {
			b = d
		} else {
			a = c
		}
		c = b - (b-a)/phi
		d = a + (b-a)/phi
	}
	return (a + b) / 2
}

// goldenMaxInt finds the maximum of f on [a, b] for integers using golden section search.
func goldenMaxInt(a, b int, f func(int) float64) int {
	for b-a > 2 {
		c := b - int(float64(b-a)/phi)
		d := a + int(float64(b-a)/phi)
		if c == d {
			d++
		}
		if f(c) > f(d) {
			b = d
		} else {
			a = c
		}
	}
	// Check all remaining candidates
	best := a
	bestVal := f(a)
	for x := a + 1; x <= b; x++ {
		if v := f(x); v > bestVal {
			best = x
			bestVal = v
		}
	}
	return best
}

func loadBars(symbol string) (*ds.Bars, error) {
	path := filepath.Join(*flagDataDir, symbol)
	return ds.OpenBars(path)
}

// simulate runs the levi algorithm and returns (sharpe, total_return)
func simulate(bars *ds.Bars, trail, mingap decimal.Decimal, lookback int) (float64, float64) {
	cash := *flagCash
	var position decimal.Decimal // shares held
	var highSince decimal.Decimal
	var dailyReturns []float64
	var dayStart decimal.Decimal
	var currentDate int
	var maxHigh *indicators.Max
	var lookbackHigh decimal.Decimal

	count := bars.Count()
	for i := 0; i < count; i++ {
		bar := bars.Get(i)
		date := bar.Timestamp.DateInt()
		time := bar.Timestamp.ClockInt()

		// Day change
		if date != currentDate {
			// Record daily return
			if !dayStart.IsZero() {
				portfolioValue := cash.Add(position.Mul(bar.Open))
				dailyRet := portfolioValue.Sub(dayStart).Div(dayStart).Float64()
				dailyReturns = append(dailyReturns, dailyRet)
			}
			// Reset for new day
			currentDate = date
			maxHigh = indicators.NewMax(clocky.Duration(lookback)*clocky.Minute - 1)
			lookbackHigh = decimal.Zero
			position = decimal.Zero
			highSince = decimal.Zero
			dayStart = cash // portfolio value at start of day (flat)
		}

		// Update rolling max indicator
		lookbackHigh = maxHigh.Value
		maxHigh.Add(bar.Timestamp, bar.High)

		if !maxHigh.IsReady() {
			continue
		}
		if time < openTime {
			continue
		}

		price := bar.Close

		// EOD close
		if time >= closeTime {
			if position.IsPositive() {
				cash = cash.Add(position.Mul(price))
				position = decimal.Zero
			}
			continue
		}

		// Check trailing stop
		if position.IsPositive() {
			if price.Cmp(highSince) > 0 {
				highSince = price
			}
			stopPrice := highSince.Mul(decimal.One.Sub(trail))
			if price.Cmp(stopPrice) <= 0 {
				cash = cash.Add(position.Mul(price))
				position = decimal.Zero
				highSince = decimal.Zero
			}
			continue
		}

		// Skip penny stocks
		if price.Cmp(*flagMinPrice) < 0 {
			continue
		}

		// Check for breakout entry
		if lookbackHigh.IsZero() {
			continue
		}

		threshold := lookbackHigh.Mul(decimal.One.Add(mingap))
		if price.Cmp(threshold) <= 0 {
			continue
		}

		// Enter position (use all cash, no margin for simplicity)
		shares := cash.Div(price).QuantizeTruncate(decimal.One)
		if shares.IsPositive() {
			position = shares
			highSince = price
			cash = cash.Sub(shares.Mul(price))
		}
	}

	// Final portfolio value
	finalValue := cash.Add(position.Mul(bars.Get(count - 1).Close))
	totalReturn := finalValue.Sub(*flagCash).Div(*flagCash).Float64()

	// Calculate Sharpe ratio (annualized)
	if len(dailyReturns) < 20 {
		return math.Inf(-1), 0
	}

	var sum, sumSq float64
	for _, r := range dailyReturns {
		sum += r
		sumSq += r * r
	}
	n := float64(len(dailyReturns))
	mean := sum / n
	variance := (sumSq / n) - (mean * mean)
	if variance <= 0 {
		return math.Inf(-1), 0
	}
	stddev := math.Sqrt(variance)

	// Annualize: 252 trading days
	annualizedReturn := mean * 252
	annualizedStddev := stddev * math.Sqrt(252)
	sharpe := annualizedReturn / annualizedStddev

	return sharpe, totalReturn
}

func writeOutput(results []Result) error {
	var buf strings.Builder

	buf.WriteString("// Code generated by go run ./cmd/levi/paramgen -o cmd/levi/params.go. DO NOT EDIT.\n\n")
	buf.WriteString("package main\n\n")
	buf.WriteString("import \"dropbear/decimal\"\n\n")
	buf.WriteString("// Params holds optimized trading parameters for a symbol.\n")
	buf.WriteString("type Params struct {\n")
	buf.WriteString("\tTrail    decimal.Decimal\n")
	buf.WriteString("\tMinGap   decimal.Decimal\n")
	buf.WriteString("\tLookback int\n")
	buf.WriteString("}\n\n")
	buf.WriteString("// OptimalParams contains grid-searched optimal parameters per symbol.\n")
	buf.WriteString("var OptimalParams = map[string]Params{\n")

	for _, r := range results {
		fmt.Fprintf(&buf, "\t%q: {Trail: decimal.Parse(%q), MinGap: decimal.Parse(%q), Lookback: %d},\n",
			r.Symbol,
			r.Trail.String(),
			r.MinGap.String(),
			r.Lookback)
	}

	buf.WriteString("}\n")

	if *flagOutput == "" {
		fmt.Print(buf.String())
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(*flagOutput)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(*flagOutput, []byte(buf.String()), 0644)
}
