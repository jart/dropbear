package main

import (
	"cmp"
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/netty"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

func init() {
	netty.SetOffline()
}

var (
	days = flag.Int("days", 200, "number of trading days to analyze")
)

func main() {
	flag.Parse()

	// Get healthy symbols
	var symbols []string
	for _, a := range alpaca.Assets {
		if a.IsHealthy() {
			symbols = append(symbols, a.Symbol.String())
		}
	}

	// Calculate cutoff time (approximate, using calendar days)
	cutoff := clocky.Now() - clocky.Time(clocky.Duration(*days)*clocky.Day*7/5) // rough trading day adjustment

	// Score each symbol
	type result struct {
		symbol        string
		monotonicity  float64 // 0-1, higher = more consistently downward
		avgNotional   float64 // average daily notional volume
		notionalScore float64 // 0-1 normalized
		combinedScore float64
	}

	var results []result
	minutesDir := ds.EquityMinutesDir()

	for _, sym := range symbols {
		path := filepath.Join(minutesDir, sym)
		bars, err := ds.OpenBars(path)
		if err != nil {
			continue // skip symbols without data
		}

		dailyBars := aggregateToDailyBars(bars, cutoff)
		bars.Close()

		if len(dailyBars) < 20 { // need enough data
			continue
		}

		mono := calcMonotonicity(dailyBars)
		avgNot := calcAvgDailyNotional(dailyBars)

		results = append(results, result{
			symbol:       sym,
			monotonicity: mono,
			avgNotional:  avgNot,
		})
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "no symbols with sufficient data")
		os.Exit(1)
	}

	// Normalize notional volume to 0-1 using rank percentile
	slices.SortFunc(results, func(a, b result) int {
		return cmp.Compare(a.avgNotional, b.avgNotional)
	})
	for i := range results {
		results[i].notionalScore = float64(i) / float64(len(results)-1)
	}

	// Calculate combined score (50/50 blend)
	for i := range results {
		results[i].combinedScore = 0.5*results[i].monotonicity + 0.5*results[i].notionalScore
	}

	// Sort by combined score descending
	slices.SortFunc(results, func(a, b result) int {
		if c := cmp.Compare(b.combinedScore, a.combinedScore); c != 0 {
			return c
		}
		return cmp.Compare(a.symbol, b.symbol)
	})

	// Print results
	fmt.Printf("%-8s %8s %8s %8s %15s\n", "SYMBOL", "MONO", "VOL", "SCORE", "AVG_NOTIONAL")
	for _, r := range results {
		fmt.Printf("%-8s %8.4f %8.4f %8.4f %15.0f\n",
			r.symbol, r.monotonicity, r.notionalScore, r.combinedScore, r.avgNotional)
	}
}

// dailyBar holds aggregated daily data
type dailyBar struct {
	date     clocky.Time
	close    decimal.Decimal
	notional decimal.Decimal // volume * vwap
}

// aggregateToDailyBars converts minute bars to daily bars
func aggregateToDailyBars(bars *ds.Bars, cutoff clocky.Time) []dailyBar {
	bars.Seek(cutoff)

	var daily []dailyBar
	var currentDay clocky.Time
	var lastClose decimal.Decimal
	var dayNotional decimal.Decimal

	for !bars.EOF() {
		bar := bars.Read()
		// Get day (truncate to midnight - approximate)
		day := bar.Timestamp - bar.Timestamp%clocky.Time(clocky.Day)

		if day != currentDay {
			if currentDay != 0 {
				daily = append(daily, dailyBar{
					date:     currentDay,
					close:    lastClose,
					notional: dayNotional,
				})
			}
			currentDay = day
			dayNotional = decimal.Zero
		}

		lastClose = bar.Close
		// notional = volume * vwap (or close if vwap not available)
		price := bar.VWAP
		if price.IsZero() {
			price = bar.Close
		}
		dayNotional = dayNotional.Add(bar.Volume.Mul(price))
	}

	// Don't forget the last day
	if currentDay != 0 {
		daily = append(daily, dailyBar{
			date:     currentDay,
			close:    lastClose,
			notional: dayNotional,
		})
	}

	return daily
}

// calcMonotonicity calculates Spearman rank correlation of price vs time
// Returns 0-1 where 1 = perfectly monotonic downward trend
func calcMonotonicity(bars []dailyBar) float64 {
	n := len(bars)
	if n < 2 {
		return 0.5
	}

	// Extract closes and create indices for ranking
	type indexed struct {
		idx   int
		close decimal.Decimal
	}
	closes := make([]indexed, n)
	for i, b := range bars {
		closes[i] = indexed{i, b.close}
	}

	// Sort by close price to get ranks
	sort.Slice(closes, func(i, j int) bool {
		return closes[i].close.Cmp(closes[j].close) < 0
	})

	// Assign ranks (1-based)
	priceRanks := make([]float64, n)
	for rank, c := range closes {
		priceRanks[c.idx] = float64(rank + 1)
	}

	// Time ranks are just 1, 2, 3, ..., n
	// Calculate Spearman correlation using the formula:
	// ρ = 1 - (6 * Σd²) / (n * (n² - 1))
	var sumD2 float64
	for i := 0; i < n; i++ {
		timeRank := float64(i + 1)
		d := priceRanks[i] - timeRank
		sumD2 += d * d
	}

	spearman := 1.0 - (6.0*sumD2)/(float64(n)*float64(n*n-1))

	// Transform: spearman=-1 (perfect downtrend) -> 1.0
	//            spearman=+1 (perfect uptrend) -> 0.0
	return (1.0 - spearman) / 2.0
}

// calcAvgDailyNotional calculates average daily notional volume
func calcAvgDailyNotional(bars []dailyBar) float64 {
	if len(bars) == 0 {
		return 0
	}
	var total decimal.Decimal
	for _, b := range bars {
		total = total.Add(b.notional)
	}
	return float64(total.Int64()) / float64(len(bars))
}
