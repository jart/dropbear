// Command paramgen finds optimal trail/mingap parameters for each stock.
//
// It grid searches over parameter combinations, scores by Sharpe ratio,
// and outputs Go code with a map of optimal parameters per symbol.
//
// Usage:
//
//	go run ./cmd/fngu/paramgen -o cmd/fngu/params.go

package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
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
)

var symbolSplitter = regexp.MustCompile(`[ \t,]+`)

var (
	flagDataDir  = flag.String("data", os.ExpandEnv("$HOME/equitydata/minutes"), "directory containing bar data")
	flagOutput   = flag.String("o", "", "output file (empty for stdout)")
	flagSymbols  = flag.String("symbols", "", "comma-separated symbols to process (empty for all)")
	flagWorkers  = flag.Int("workers", runtime.NumCPU(), "number of parallel workers")
	flagCash     = decimal.Flag("cash", "100_000", "initial cash for simulation")
	flagMinPrice = decimal.Flag("minprice", "10", "minimum share price")
	flagVerbose  = flag.Bool("v", false, "verbose output")
)

// Grid search ranges
var (
	trailValues = []decimal.Decimal{
		decimal.Parse("0.005"), // 0.5%
		decimal.Parse("0.01"),  // 1%
		decimal.Parse("0.015"), // 1.5%
		decimal.Parse("0.02"),  // 2%
		decimal.Parse("0.025"), // 2.5%
		decimal.Parse("0.03"),  // 3%
		decimal.Parse("0.04"),  // 4%
		decimal.Parse("0.05"),  // 5%
		decimal.Parse("0.06"),  // 6%
	}
	mingapValues = []decimal.Decimal{
		decimal.Parse("0.005"),  // 0.5%
		decimal.Parse("0.0075"), // 0.75%
		decimal.Parse("0.01"),   // 1%
		decimal.Parse("0.015"),  // 1.5%
		decimal.Parse("0.02"),   // 2%
		decimal.Parse("0.025"),  // 2.5%
		decimal.Parse("0.03"),   // 3%
		decimal.Parse("0.035"),  // 3.5%
		decimal.Parse("0.04"),   // 4%
		decimal.Parse("0.05"),   // 5%
		decimal.Parse("0.06"),   // 6%
	}
)

const (
	openTime  = 6_45_00
	closeTime = 12_45_00
	lookback  = 10
)

// Result holds the optimal parameters for a symbol
type Result struct {
	Symbol string
	Trail  decimal.Decimal
	MinGap decimal.Decimal
	Sharpe float64
	Return float64
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

	for _, sym := range symbols {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := optimizeSymbol(symbol)
			if err != nil {
				if *flagVerbose {
					log.Printf("%s: %v", symbol, err)
				}
				return
			}
			results <- result
			if *flagVerbose {
				log.Printf("%s: trail=%.1f%% mingap=%.2f%% sharpe=%.2f return=%.1f%%",
					symbol,
					result.Trail.MulInt(100).Float64(),
					result.MinGap.MulInt(100).Float64(),
					result.Sharpe,
					result.Return*100)
			}
		}(sym)
	}

	go func() {
		wg.Wait()
		close(results)
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

	log.Printf("Optimized %d symbols", len(all))

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

	var best Result
	best.Symbol = symbol
	best.Sharpe = math.Inf(-1)

	// Grid search
	for _, trail := range trailValues {
		for _, mingap := range mingapValues {
			sharpe, ret := simulate(bars, trail, mingap)
			if sharpe > best.Sharpe {
				best.Trail = trail
				best.MinGap = mingap
				best.Sharpe = sharpe
				best.Return = ret
			}
		}
	}

	if math.IsInf(best.Sharpe, -1) {
		return Result{}, fmt.Errorf("no valid results")
	}

	return best, nil
}

func loadBars(symbol string) (*alpaca.Bars, error) {
	path := filepath.Join(*flagDataDir, symbol)
	return alpaca.OpenBars(path)
}

// simulate runs the fngu algorithm and returns (sharpe, total_return)
func simulate(bars *alpaca.Bars, trail, mingap decimal.Decimal) (float64, float64) {
	cash := *flagCash
	var position decimal.Decimal // shares held
	var highSince decimal.Decimal
	var dailyReturns []float64
	var dayStart decimal.Decimal
	var currentDate int
	windowStart := 0

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
			windowStart = i
			position = decimal.Zero
			highSince = decimal.Zero
			dayStart = cash // portfolio value at start of day (flat)
		}

		// Window is bars[windowStart:i+1], we need at least lookback bars
		windowLen := i - windowStart + 1
		if windowLen > lookback {
			windowStart = i - lookback + 1
			windowLen = lookback
		}

		if windowLen < lookback {
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

		// Check for breakout entry (lookback high excluding current bar)
		var lookbackHigh decimal.Decimal
		for j := windowStart; j < i; j++ {
			h := bars.Get(j).High
			if lookbackHigh.IsZero() || h.Cmp(lookbackHigh) > 0 {
				lookbackHigh = h
			}
		}
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

	buf.WriteString("// Code generated by go run ./cmd/fngu/paramgen -o cmd/fngu/params.go. DO NOT EDIT.\n\n")
	buf.WriteString("package main\n\n")
	buf.WriteString("import \"dropbear/decimal\"\n\n")
	buf.WriteString("// Params holds optimized trading parameters for a symbol.\n")
	buf.WriteString("type Params struct {\n")
	buf.WriteString("\tTrail  decimal.Decimal\n")
	buf.WriteString("\tMinGap decimal.Decimal\n")
	buf.WriteString("}\n\n")
	buf.WriteString("// OptimalParams contains grid-searched optimal parameters per symbol.\n")
	buf.WriteString("var OptimalParams = map[string]Params{\n")

	for _, r := range results {
		buf.WriteString(fmt.Sprintf("\t%q: {Trail: decimal.Parse(%q), MinGap: decimal.Parse(%q)},\n",
			r.Symbol,
			r.Trail.String(),
			r.MinGap.String()))
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
