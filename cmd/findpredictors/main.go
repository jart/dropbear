// Command findpredictors discovers which equities have predictive power for
// other equities using Granger causality tests. It aggregates minute bars into
// daily log returns, fits bivariate vector autoregressive (VAR) models, and
// computes F-statistics to test whether lagged returns of stock X improve the
// forecast of stock Y beyond Y's own lags.
//
// Usage:
//
//	go run ./cmd/findpredictors -symbol etc/picks/ndx
//	go run ./cmd/findpredictors -symbol "AAPL GOOG MSFT NVDA"
//	go run ./cmd/findpredictors -days 120 -maxlag 5 -pvalue 0.01 -workers 30
package main

import (
	"dropbear/clocky"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"dropbear/netty"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func init() {
	netty.SetOffline()
}

var (
	flagSymbol  = flag.String("symbol", "etc/picks/ndx", "symbols or file to analyze")
	flagDays    = flag.Int("days", 252, "trading days of history to use")
	flagMaxLag  = flag.Int("maxlag", 5, "maximum lag order for Granger test (trading days)")
	flagPValue  = flag.Float64("pvalue", 0.05, "p-value threshold for significance")
	flagWorkers = flag.Int("workers", 0, "number of parallel workers (0 = NumCPU)")
	flagTopN    = flag.Int("topn", 50, "number of top results to display")
	flagMinVol  = flag.Float64("minvol", 1e6, "minimum average daily notional volume")
	flagSplit   = flag.Bool("split", false, "split-sample validation: only report pairs significant in both halves")
)

func main() {
	flag.Parse()

	if *flagWorkers <= 0 {
		*flagWorkers = runtime.NumCPU()
	}

	// Parse symbols
	symbols, err := symbol.Expand(*flagSymbol)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error expanding symbols: %v\n", err)
		os.Exit(1)
	}
	if len(symbols) == 0 {
		fmt.Fprintln(os.Stderr, "no symbols specified")
		os.Exit(1)
	}
	symNames := make([]string, len(symbols))
	for i, s := range symbols {
		symNames[i] = s.String()
	}
	sort.Strings(symNames)

	fmt.Fprintf(os.Stderr, "loading %d symbols...\n", len(symNames))

	// Load daily log returns for all symbols in parallel
	cutoff := clocky.Now() - clocky.Time(clocky.Duration(*flagDays)*clocky.Day*7/5)
	minutesDir := ds.EquityMinutesDir()

	type stockData struct {
		name    string
		returns []float64 // daily log returns
		dates   []clocky.Time
	}

	var mu sync.Mutex
	var stocks []stockData

	// Load bars in parallel
	var wg sync.WaitGroup
	sem := make(chan struct{}, *flagWorkers)
	for _, name := range symNames {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			dates, returns := loadDailyLogReturns(filepath.Join(minutesDir, name), cutoff)
			if len(returns) < *flagMaxLag+30 {
				return
			}
			// Check minimum volume
			avgVol := calcAvgNotional(filepath.Join(minutesDir, name), cutoff)
			if avgVol < *flagMinVol {
				return
			}
			mu.Lock()
			stocks = append(stocks, stockData{name: name, returns: returns, dates: dates})
			mu.Unlock()
		}(name)
	}
	wg.Wait()

	sort.Slice(stocks, func(i, j int) bool {
		return stocks[i].name < stocks[j].name
	})

	fmt.Fprintf(os.Stderr, "loaded %d stocks with sufficient data\n", len(stocks))
	fmt.Fprintf(os.Stderr, "testing %d pairs with maxlag=%d...\n",
		len(stocks)*(len(stocks)-1), *flagMaxLag)

	// Align all return series to common dates
	// Build a union date set, then for each stock create an aligned series
	dateSet := make(map[clocky.Time]struct{})
	for _, s := range stocks {
		for _, d := range s.dates {
			dateSet[d] = struct{}{}
		}
	}
	allDates := make([]clocky.Time, 0, len(dateSet))
	for d := range dateSet {
		allDates = append(allDates, d)
	}
	slices.Sort(allDates)

	// Create date-to-index map
	dateIdx := make(map[clocky.Time]int, len(allDates))
	for i, d := range allDates {
		dateIdx[d] = i
	}

	// Align each stock's returns to the common timeline (NaN for missing)
	names := make([]string, len(stocks))
	aligned := make([][]float64, len(stocks))
	for i, s := range stocks {
		names[i] = s.name
		r := make([]float64, len(allDates))
		for j := range r {
			r[j] = math.NaN()
		}
		for j, d := range s.dates {
			if idx, ok := dateIdx[d]; ok {
				r[idx] = s.returns[j]
			}
		}
		aligned[i] = r
	}

	if *flagSplit {
		runSplitSample(names, aligned, sem)
	} else {
		results := runScan("full", names, aligned, sem)
		printResults(results, len(names))
	}
}

// grangerResult holds the output of a single Granger causality test.
type grangerResult struct {
	predictor string
	target    string
	fstat     float64
	pvalue    float64
	bestLag   int
	corrSign  float64
}

// runScan runs Granger causality on all directed pairs and returns significant results.
func runScan(label string, names []string, aligned [][]float64, sem chan struct{}) []grangerResult {
	type pair struct{ i, j int }
	var pairs []pair
	for i := 0; i < len(aligned); i++ {
		for j := 0; j < len(aligned); j++ {
			if i != j {
				pairs = append(pairs, pair{i, j})
			}
		}
	}

	fmt.Fprintf(os.Stderr, "[%s] testing %d pairs with maxlag=%d...\n",
		label, len(pairs), *flagMaxLag)

	var done atomic.Int64
	total := int64(len(pairs))

	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d := done.Load()
				fmt.Fprintf(os.Stderr, "[%s] progress: %d/%d (%.1f%%)\n",
					label, d, total, 100*float64(d)/float64(total))
			case <-progressDone:
				return
			}
		}
	}()

	var results []grangerResult
	var resultsMu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range pairs {
		wg.Add(1)
		go func(p pair) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fstat, pval, bestLag, corrSign := grangerCausality(aligned[p.i], aligned[p.j], *flagMaxLag)
			done.Add(1)
			if pval < *flagPValue {
				resultsMu.Lock()
				results = append(results, grangerResult{
					predictor: names[p.i],
					target:    names[p.j],
					fstat:     fstat,
					pvalue:    pval,
					bestLag:   bestLag,
					corrSign:  corrSign,
				})
				resultsMu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	close(progressDone)

	fmt.Fprintf(os.Stderr, "[%s] %d significant pairs (p < %g) out of %d tested\n",
		label, len(results), *flagPValue, len(pairs))

	sort.Slice(results, func(i, j int) bool {
		return results[i].fstat > results[j].fstat
	})
	return results
}

// runSplitSample splits the timeline in half, runs Granger on each half
// independently, and reports only pairs significant in both halves.
func runSplitSample(names []string, aligned [][]float64, sem chan struct{}) {
	n := len(aligned[0])
	mid := n / 2
	fmt.Fprintf(os.Stderr, "split-sample: %d total days, half1=%d days, half2=%d days\n",
		n, mid, n-mid)

	// Slice each stock's returns into two halves
	half1 := make([][]float64, len(aligned))
	half2 := make([][]float64, len(aligned))
	for i, r := range aligned {
		half1[i] = r[:mid]
		half2[i] = r[mid:]
	}

	results1 := runScan("half1", names, half1, sem)
	results2 := runScan("half2", names, half2, sem)

	// Index half2 results by (predictor, target) for lookup
	type pairKey struct{ predictor, target string }
	h2 := make(map[pairKey]grangerResult, len(results2))
	for _, r := range results2 {
		h2[pairKey{r.predictor, r.target}] = r
	}

	// Find pairs significant in both halves
	type stableResult struct {
		predictor string
		target    string
		f1, f2    float64
		p1, p2    float64
		lag1      int
		lag2      int
		sign1     float64
		sign2     float64
		sameSign  bool
	}

	var stable []stableResult
	for _, r1 := range results1 {
		r2, ok := h2[pairKey{r1.predictor, r1.target}]
		if !ok {
			continue
		}
		stable = append(stable, stableResult{
			predictor: r1.predictor,
			target:    r1.target,
			f1:        r1.fstat,
			f2:        r2.fstat,
			p1:        r1.pvalue,
			p2:        r2.pvalue,
			lag1:      r1.bestLag,
			lag2:      r2.bestLag,
			sign1:     r1.corrSign,
			sign2:     r2.corrSign,
			sameSign:  r1.corrSign == r2.corrSign,
		})
	}

	// Sort by min(F1, F2) descending — the pair is only as strong as its weaker half
	sort.Slice(stable, func(i, j int) bool {
		return min(stable[i].f1, stable[i].f2) > min(stable[j].f1, stable[j].f2)
	})

	n2 := *flagTopN
	if n2 > len(stable) {
		n2 = len(stable)
	}

	fmt.Printf("\nSplit-sample stable pairs (significant in BOTH halves):\n")
	fmt.Printf("%-8s -> %-8s  %7s %7s  %9s %9s  %4s %4s  %4s\n",
		"PREDICT", "TARGET", "F1", "F2", "P1", "P2", "LAG1", "LAG2", "SIGN")
	fmt.Println(strings.Repeat("-", 75))
	for i := 0; i < n2; i++ {
		r := stable[i]
		sign := "SAME"
		if !r.sameSign {
			sign = "FLIP"
		}
		s1 := "+"
		if r.sign1 < 0 {
			s1 = "-"
		}
		fmt.Printf("%-8s -> %-8s  %7.1f %7.1f  %9.1e %9.1e  %4d %4d  %s(%s)\n",
			r.predictor, r.target, r.f1, r.f2, r.p1, r.p2, r.lag1, r.lag2, sign, s1)
	}

	// Persistence stats
	fmt.Fprintf(os.Stderr, "\npersistence: %d of %d half1 pairs survived in half2 (%.1f%%)\n",
		len(stable), len(results1), 100*float64(len(stable))/max(float64(len(results1)), 1))
	expected := float64(len(results1)) * float64(len(results2)) / float64(len(names)*(len(names)-1))
	fmt.Fprintf(os.Stderr, "expected by chance: %.0f pairs (%.1f%%)\n",
		expected, 100*expected/max(float64(len(results1)), 1))

	// Predictor/target summary for stable pairs only
	predCount := make(map[string]int)
	targCount := make(map[string]int)
	for _, r := range stable {
		predCount[r.predictor]++
		targCount[r.target]++
	}
	printCountSummary("Stable predictors", predCount)
	printCountSummary("Stably predictable", targCount)
}

// printResults prints the standard (non-split) output.
func printResults(results []grangerResult, nStocks int) {
	n := *flagTopN
	if n > len(results) {
		n = len(results)
	}

	fmt.Printf("\n%-8s -> %-8s  %8s  %10s  %4s  %5s\n",
		"PREDICT", "TARGET", "F-STAT", "P-VALUE", "LAG", "SIGN")
	fmt.Println(strings.Repeat("-", 55))
	for i := 0; i < n; i++ {
		r := results[i]
		sign := "+"
		if r.corrSign < 0 {
			sign = "-"
		}
		fmt.Printf("%-8s -> %-8s  %8.2f  %10.2e  %4d  %5s\n",
			r.predictor, r.target, r.fstat, r.pvalue, r.bestLag, sign)
	}

	predCount := make(map[string]int)
	targCount := make(map[string]int)
	for _, r := range results {
		predCount[r.predictor]++
		targCount[r.target]++
	}
	printCountSummary("Top predictors", predCount)
	printCountSummary("Most predictable", targCount)
}

func printCountSummary(title string, counts map[string]int) {
	type entry struct {
		name  string
		count int
	}
	var rank []entry
	for k, v := range counts {
		rank = append(rank, entry{k, v})
	}
	sort.Slice(rank, func(i, j int) bool {
		return rank[i].count > rank[j].count
	})
	fmt.Printf("\n%s:\n", title)
	for i := 0; i < min(15, len(rank)); i++ {
		fmt.Printf("  %-8s  %3d\n", rank[i].name, rank[i].count)
	}
}

// loadDailyLogReturns loads minute bars, aggregates to daily closes, and
// computes log returns. Log returns are additive across time and closer to
// normally distributed than simple returns, which makes them better for
// regression-based Granger tests.
func loadDailyLogReturns(path string, cutoff clocky.Time) ([]clocky.Time, []float64) {
	bars, err := ds.OpenBars(path)
	if err != nil {
		return nil, nil
	}
	defer bars.Close()
	bars.Seek(cutoff)

	// Aggregate to daily closes
	type dayClose struct {
		date  clocky.Time
		close float64
	}
	var days []dayClose
	var currentDay clocky.Time
	var lastClose float64

	for !bars.EOF() {
		bar := bars.Read()
		day := bar.Timestamp - bar.Timestamp%clocky.Time(clocky.Day)
		if day != currentDay {
			if currentDay != 0 && lastClose > 0 {
				days = append(days, dayClose{date: currentDay, close: lastClose})
			}
			currentDay = day
		}
		lastClose = bar.Close.Float64()
	}
	if currentDay != 0 && lastClose > 0 {
		days = append(days, dayClose{date: currentDay, close: lastClose})
	}

	if len(days) < 2 {
		return nil, nil
	}

	// Compute log returns
	dates := make([]clocky.Time, len(days)-1)
	returns := make([]float64, len(days)-1)
	for i := 1; i < len(days); i++ {
		dates[i-1] = days[i].date
		returns[i-1] = math.Log(days[i].close / days[i-1].close)
	}
	return dates, returns
}

// calcAvgNotional computes average daily notional volume for filtering.
func calcAvgNotional(path string, cutoff clocky.Time) float64 {
	bars, err := ds.OpenBars(path)
	if err != nil {
		return 0
	}
	defer bars.Close()
	bars.Seek(cutoff)

	var totalNotional float64
	var currentDay clocky.Time
	var dayNotional float64
	var numDays int

	for !bars.EOF() {
		bar := bars.Read()
		day := bar.Timestamp - bar.Timestamp%clocky.Time(clocky.Day)
		if day != currentDay {
			if currentDay != 0 {
				totalNotional += dayNotional
				numDays++
			}
			currentDay = day
			dayNotional = 0
		}
		price := bar.VWAP.Float64()
		if price == 0 {
			price = bar.Close.Float64()
		}
		dayNotional += bar.Volume.Float64() * price
	}
	if currentDay != 0 {
		totalNotional += dayNotional
		numDays++
	}
	if numDays == 0 {
		return 0
	}
	return totalNotional / float64(numDays)
}

// grangerCausality tests whether time series x Granger-causes time series y.
//
// The test fits two OLS models:
//
//	Restricted:   y(t) = a0 + a1*y(t-1) + ... + ap*y(t-p) + e
//	Unrestricted: y(t) = a0 + a1*y(t-1) + ... + ap*y(t-p) + b1*x(t-1) + ... + bp*x(t-p) + e
//
// and computes the F-statistic:
//
//	F = ((RSS_r - RSS_u) / p) / (RSS_u / (n - 2p - 1))
//
// where p = lag order and n = number of observations.
//
// We test all lag orders from 1..maxLag and pick the one with the best
// (lowest) p-value, applying a Bonferroni correction for multiple testing.
//
// Returns: best F-stat, corrected p-value, best lag, correlation sign at best lag.
func grangerCausality(x, y []float64, maxLag int) (float64, float64, int, float64) {
	n := len(x)
	if n != len(y) {
		// Use shorter length
		if len(y) < n {
			n = len(y)
		}
		x = x[len(x)-n:]
		y = y[len(y)-n:]
	}

	bestF := 0.0
	bestP := 1.0
	bestLag := 1
	bestSign := 0.0

	for lag := 1; lag <= maxLag; lag++ {
		f, p, sign := grangerTest(x, y, lag)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			continue
		}
		// Bonferroni correction for testing multiple lags
		corrP := p * float64(maxLag)
		if corrP > 1 {
			corrP = 1
		}
		if corrP < bestP {
			bestF = f
			bestP = corrP
			bestLag = lag
			bestSign = sign
		}
	}

	return bestF, bestP, bestLag, bestSign
}

// grangerTest performs the Granger test at a specific lag order.
// Handles NaN values by skipping those observations.
func grangerTest(x, y []float64, lag int) (float64, float64, float64) {
	n := len(x)
	if n <= 2*lag+1 {
		return math.NaN(), 1, 0
	}

	// Build observation vectors, skipping NaN
	var observations []obs
	for t := lag; t < n; t++ {
		if math.IsNaN(y[t]) {
			continue
		}
		valid := true
		yLags := make([]float64, lag)
		xLags := make([]float64, lag)
		for k := 1; k <= lag; k++ {
			if math.IsNaN(y[t-k]) || math.IsNaN(x[t-k]) {
				valid = false
				break
			}
			yLags[k-1] = y[t-k]
			xLags[k-1] = x[t-k]
		}
		if !valid {
			continue
		}
		observations = append(observations, obs{yTarget: y[t], yLags: yLags, xLags: xLags})
	}

	nObs := len(observations)
	if nObs <= 2*lag+1 {
		return math.NaN(), 1, 0
	}

	// Restricted model: y(t) = c + sum(a_k * y(t-k))
	// Number of parameters: lag + 1 (intercept)
	rssR := olsResidualSS(observations, lag, false)

	// Unrestricted model: y(t) = c + sum(a_k * y(t-k)) + sum(b_k * x(t-k))
	// Number of parameters: 2*lag + 1 (intercept)
	rssU, bCoeffs := olsResidualSSWithCoeffs(observations, lag, true)

	if rssU <= 0 || rssR < rssU {
		return 0, 1, 0
	}

	// F-statistic
	p := float64(lag)
	dfU := float64(nObs) - 2*p - 1
	if dfU <= 0 {
		return math.NaN(), 1, 0
	}
	f := ((rssR - rssU) / p) / (rssU / dfU)

	// p-value from F-distribution using regularized incomplete beta function
	pval := fDistPValue(f, int(p), int(dfU))

	// Sign of the strongest x coefficient
	sign := 0.0
	maxAbs := 0.0
	for _, b := range bCoeffs {
		if math.Abs(b) > maxAbs {
			maxAbs = math.Abs(b)
			sign = b
		}
	}
	if sign > 0 {
		sign = 1
	} else if sign < 0 {
		sign = -1
	}

	return f, pval, sign
}

// olsResidualSS computes residual sum of squares for OLS regression.
// Uses normal equations: beta = (X'X)^{-1} X'y
func olsResidualSS(observations []obs, lag int, includeX bool) float64 {
	ss, _ := olsResidualSSWithCoeffs(observations, lag, includeX)
	return ss
}

type obs struct {
	yTarget float64
	yLags   []float64
	xLags   []float64
}

// olsResidualSSWithCoeffs computes RSS and returns the x-lag coefficients.
func olsResidualSSWithCoeffs(observations []obs, lag int, includeX bool) (float64, []float64) {
	nObs := len(observations)
	nCols := lag + 1 // intercept + y lags
	if includeX {
		nCols += lag // + x lags
	}

	// Build X matrix (row-major) and y vector
	X := make([]float64, nObs*nCols)
	yVec := make([]float64, nObs)

	for i, o := range observations {
		row := i * nCols
		X[row] = 1 // intercept
		for k := 0; k < lag; k++ {
			X[row+1+k] = o.yLags[k]
		}
		if includeX {
			for k := 0; k < lag; k++ {
				X[row+1+lag+k] = o.xLags[k]
			}
		}
		yVec[i] = o.yTarget
	}

	// Compute X'X (nCols x nCols)
	XtX := make([]float64, nCols*nCols)
	for i := 0; i < nCols; i++ {
		for j := i; j < nCols; j++ {
			var sum float64
			for k := 0; k < nObs; k++ {
				sum += X[k*nCols+i] * X[k*nCols+j]
			}
			XtX[i*nCols+j] = sum
			XtX[j*nCols+i] = sum
		}
	}

	// Compute X'y (nCols)
	Xty := make([]float64, nCols)
	for i := 0; i < nCols; i++ {
		var sum float64
		for k := 0; k < nObs; k++ {
			sum += X[k*nCols+i] * yVec[k]
		}
		Xty[i] = sum
	}

	// Solve X'X * beta = X'y using Cholesky decomposition
	beta := choleskySolve(XtX, Xty, nCols)
	if beta == nil {
		// Fall back to computing just mean prediction
		var mean float64
		for _, y := range yVec {
			mean += y
		}
		mean /= float64(nObs)
		var ss float64
		for _, y := range yVec {
			d := y - mean
			ss += d * d
		}
		return ss, nil
	}

	// Compute residual sum of squares
	var rss float64
	for i := 0; i < nObs; i++ {
		var yHat float64
		row := i * nCols
		for j := 0; j < nCols; j++ {
			yHat += X[row+j] * beta[j]
		}
		resid := yVec[i] - yHat
		rss += resid * resid
	}

	// Extract x coefficients
	var xCoeffs []float64
	if includeX {
		xCoeffs = beta[1+lag : 1+2*lag]
	}

	return rss, xCoeffs
}

// choleskySolve solves A*x = b where A is symmetric positive definite using
// Cholesky decomposition. Returns nil if decomposition fails.
func choleskySolve(A, b []float64, n int) []float64 {
	// Cholesky decomposition: A = L * L^T
	L := make([]float64, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			var sum float64
			for k := 0; k < j; k++ {
				sum += L[i*n+k] * L[j*n+k]
			}
			if i == j {
				val := A[i*n+i] - sum
				if val <= 0 {
					return nil // not positive definite
				}
				L[i*n+j] = math.Sqrt(val)
			} else {
				if L[j*n+j] == 0 {
					return nil
				}
				L[i*n+j] = (A[i*n+j] - sum) / L[j*n+j]
			}
		}
	}

	// Forward substitution: L*y = b
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		var sum float64
		for k := 0; k < i; k++ {
			sum += L[i*n+k] * y[k]
		}
		y[i] = (b[i] - sum) / L[i*n+i]
	}

	// Back substitution: L^T * x = y
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		var sum float64
		for k := i + 1; k < n; k++ {
			sum += L[k*n+i] * x[k]
		}
		x[i] = (y[i] - sum) / L[i*n+i]
	}

	return x
}

// fDistPValue computes the upper tail probability P(F > f) for the
// F-distribution with d1 and d2 degrees of freedom using the regularized
// incomplete beta function.
//
//	P(F > f) = I_{d2/(d2+d1*f)}(d2/2, d1/2)
func fDistPValue(f float64, d1, d2 int) float64 {
	if f <= 0 || d1 <= 0 || d2 <= 0 {
		return 1
	}
	x := float64(d2) / (float64(d2) + float64(d1)*f)
	return regularizedBeta(x, float64(d2)/2, float64(d1)/2)
}

// regularizedBeta computes the regularized incomplete beta function I_x(a,b)
// using the continued fraction expansion (Lentz's method).
func regularizedBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}

	// Use symmetry relation if x > (a+1)/(a+b+2) for better convergence
	if x > (a+1)/(a+b+2) {
		return 1 - regularizedBeta(1-x, b, a)
	}

	// Log of the prefactor: x^a * (1-x)^b / (a * Beta(a,b))
	lnPre := a*math.Log(x) + b*math.Log(1-x) -
		math.Log(a) - lnBeta(a, b)

	// Continued fraction (Lentz's modified method)
	const maxIter = 200
	const eps = 1e-14
	const tiny = 1e-30

	// f = 1, C = 1, D = 1/(1 - (a+b)*x/(a+1))
	qab := a + b
	qap := a + 1
	qam := a - 1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1 / d
	h := d

	for m := 1; m <= maxIter; m++ {
		fm := float64(m)
		// Even step
		num := fm * (b - fm) * x / ((qam + 2*fm) * (a + 2*fm))
		d = 1 + num*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + num/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		h *= d * c

		// Odd step
		num = -(a + fm) * (qab + fm) * x / ((a + 2*fm) * (qap + 2*fm))
		d = 1 + num*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + num/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		delta := d * c
		h *= delta

		if math.Abs(delta-1) < eps {
			break
		}
	}

	return math.Exp(lnPre) * h
}

// lnBeta computes ln(Beta(a,b)) = lnGamma(a) + lnGamma(b) - lnGamma(a+b)
func lnBeta(a, b float64) float64 {
	la, _ := math.Lgamma(a)
	lb, _ := math.Lgamma(b)
	lab, _ := math.Lgamma(a + b)
	return la + lb - lab
}
