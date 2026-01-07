package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	flagStartYear = flag.Int("year", 2020, "start year for testing (tests 6-month periods from this year to now)")
	flagCash      = flag.String("cash", "161844", "starting cash")
	flagTrail     = flag.String("trail", "", "trail stop percentage (optional)")
	flagMingap    = flag.String("mingap", "", "minimum gap percentage (optional)")
	flagLookback  = flag.String("lookback", "10", "lookback period in minutes")
	flagRekt      = flag.String("rekt", "", "liquidation threshold (optional)")
	flagPicks     = flag.String("picks", "varu", "picks file name in etc/picks/ (default: varu)")
	flagWorkers   = flag.Int("workers", 0, "number of parallel workers (default: CPU/2)")
	flagVerbose   = flag.Bool("v", false, "verbose output")
)

type periodResult struct {
	period        string
	annualizedPct float64
	baselinePct   float64
	deltaPct      float64
	liquidated    bool
	err           error
}

type stockResult struct {
	symbol       string
	periods      []periodResult
	avgDelta     float64
	minDelta     float64
	maxDelta     float64
	consistency  int // number of periods where removal helped (positive delta)
	liquidations int // number of periods where baseline got liquidated
	totalPeriods int
}

type job struct {
	symbol    string
	startDate string
	endDate   string
	period    string
	symbols   []string // full list minus this symbol
}

func main() {
	flag.Parse()

	// Load standard varu picks
	standardPicks, err := loadStandardPicks()
	if err != nil {
		log.Fatalf("failed to load standard picks: %v", err)
	}
	log.Printf("Loaded %d standard picks from etc/picks/%s", len(standardPicks), *flagPicks)

	if len(standardPicks) < 2 {
		log.Fatal("need at least 2 stocks to test removal")
	}

	// Generate 6-month periods from start year to now
	periods := generatePeriods(*flagStartYear)
	log.Printf("Testing %d 6-month periods from %d to now", len(periods), *flagStartYear)

	// Build varu binary
	varuBinary, err := buildVaru()
	if err != nil {
		log.Fatalf("failed to build varu: %v", err)
	}
	defer os.Remove(varuBinary)
	log.Printf("Built varu binary: %s", varuBinary)

	// Determine worker count
	workers := *flagWorkers
	if workers <= 0 {
		workers = runtime.NumCPU() / 2
		if workers < 1 {
			workers = 1
		}
	}
	log.Printf("Using %d workers", workers)

	// First, run baselines for each period (full list)
	log.Printf("Running baselines for each period...")
	baselines := make(map[string]float64)
	baselineLiquidated := make(map[string]bool)
	for _, p := range periods {
		result := runBacktest(varuBinary, standardPicks, p.start, p.end)
		if result.err != nil {
			log.Printf("  %s: ERROR: %v", p.label, result.err)
			baselines[p.label] = 0
		} else if result.liquidated {
			log.Printf("  %s: LIQUIDATED (baseline got rekt)", p.label)
			baselines[p.label] = -100
			baselineLiquidated[p.label] = true
		} else {
			log.Printf("  %s: %.2f%% annualized", p.label, result.annualizedPct)
			baselines[p.label] = result.annualizedPct
		}
	}

	// Filter to only periods with valid baselines
	var validPeriods []period
	for _, p := range periods {
		if _, ok := baselines[p.label]; ok {
			validPeriods = append(validPeriods, p)
		}
	}
	if len(validPeriods) == 0 {
		log.Fatal("no valid periods (all baselines failed)")
	}
	log.Printf("Testing %d valid periods (skipped %d with errors)", len(validPeriods), len(periods)-len(validPeriods))

	// Generate all jobs: for each stock × each valid period
	var jobs []job
	for _, symbol := range standardPicks {
		// Create list without this symbol
		var symbolsWithout []string
		for _, s := range standardPicks {
			if s != symbol {
				symbolsWithout = append(symbolsWithout, s)
			}
		}
		for _, p := range validPeriods {
			jobs = append(jobs, job{
				symbol:    symbol,
				startDate: p.start,
				endDate:   p.end,
				period:    p.label,
				symbols:   symbolsWithout,
			})
		}
	}

	log.Printf("Running %d tests (%d stocks × %d periods)...", len(jobs), len(standardPicks), len(validPeriods))

	// Run jobs in parallel
	jobChan := make(chan job, len(jobs))
	type jobResult struct {
		symbol string
		period string
		result backtestResult
	}
	resultChan := make(chan jobResult, len(jobs))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobChan {
				r := runBacktest(varuBinary, j.symbols, j.startDate, j.endDate)
				resultChan <- jobResult{
					symbol: j.symbol,
					period: j.period,
					result: r,
				}
			}
		}()
	}

	// Send jobs
	for _, j := range jobs {
		jobChan <- j
	}
	close(jobChan)

	// Wait and close results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	stockResults := make(map[string]*stockResult)
	for _, symbol := range standardPicks {
		stockResults[symbol] = &stockResult{
			symbol:  symbol,
			periods: make([]periodResult, 0, len(periods)),
		}
	}

	completed := 0
	total := len(jobs)
	for jr := range resultChan {
		completed++
		baseline := baselines[jr.period]
		delta := jr.result.annualizedPct - baseline // positive = removing helped

		// Check if removing this stock prevented a baseline liquidation
		// (baseline was liquidated, but without this stock it wasn't)
		causedRekt := baselineLiquidated[jr.period] && !jr.result.liquidated

		pr := periodResult{
			period:        jr.period,
			annualizedPct: jr.result.annualizedPct,
			baselinePct:   baseline,
			deltaPct:      delta,
			liquidated:    causedRekt, // true if this stock caused baseline to get rekt
			err:           jr.result.err,
		}
		stockResults[jr.symbol].periods = append(stockResults[jr.symbol].periods, pr)

		if *flagVerbose {
			if jr.result.err != nil {
				log.Printf("[%d/%d] -%s %s: ERROR", completed, total, jr.symbol, jr.period)
			} else if causedRekt {
				log.Printf("[%d/%d] -%s %s: %.2f%% (PREVENTED REKT)", completed, total, jr.symbol, jr.period, jr.result.annualizedPct)
			} else if jr.result.liquidated {
				log.Printf("[%d/%d] -%s %s: STILL LIQUIDATED", completed, total, jr.symbol, jr.period)
			} else {
				sign := "+"
				if delta < 0 {
					sign = ""
				}
				log.Printf("[%d/%d] -%s %s: %.2f%% (%s%.2f%%)", completed, total, jr.symbol, jr.period, jr.result.annualizedPct, sign, delta)
			}
		} else if completed%100 == 0 || completed == total {
			log.Printf("[%d/%d] tests completed", completed, total)
		}
	}

	// Calculate aggregate stats for each stock
	var results []*stockResult
	for _, sr := range stockResults {
		sr.totalPeriods = len(sr.periods)
		if sr.totalPeriods == 0 {
			continue
		}

		var sum float64
		sr.minDelta = sr.periods[0].deltaPct
		sr.maxDelta = sr.periods[0].deltaPct
		for _, pr := range sr.periods {
			if pr.liquidated {
				sr.liquidations++
			}
			if pr.err == nil {
				sum += pr.deltaPct
				if pr.deltaPct > sr.maxDelta {
					sr.maxDelta = pr.deltaPct
				}
				if pr.deltaPct < sr.minDelta {
					sr.minDelta = pr.deltaPct
				}
				if pr.deltaPct > 0 {
					sr.consistency++
				}
			}
		}
		sr.avgDelta = sum / float64(sr.totalPeriods)
		results = append(results, sr)
	}

	// Sort by average delta (stocks that hurt most when present = highest avgDelta when removed)
	sort.Slice(results, func(i, j int) bool {
		return results[i].avgDelta > results[j].avgDelta
	})

	// Print report
	fmt.Println()
	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("VETO REPORT: Effect of removing each stock (positive delta = removal helped)")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("%-10s %10s %10s %10s %12s %6s %s\n", "SYMBOL", "AVG DELTA", "MIN", "MAX", "CONSISTENCY", "REKT", "VERDICT")
	fmt.Println(strings.Repeat("-", 100))

	var vetoList, keepList, neutralList []string
	for _, sr := range results {
		verdict := "KEEP"
		if sr.liquidations > 0 {
			verdict = "VETO (REKT)"
			vetoList = append(vetoList, sr.symbol)
		} else if sr.avgDelta > 5.0 && sr.consistency > sr.totalPeriods/2 {
			verdict = "VETO"
			vetoList = append(vetoList, sr.symbol)
		} else if sr.avgDelta < -5.0 {
			verdict = "STRONG KEEP"
			keepList = append(keepList, sr.symbol)
		} else {
			neutralList = append(neutralList, sr.symbol)
		}

		rektStr := ""
		if sr.liquidations > 0 {
			rektStr = fmt.Sprintf("%d", sr.liquidations)
		}
		fmt.Printf("%-10s %+9.2f%% %+9.2f%% %+9.2f%% %5d/%-5d   %5s  %s\n",
			sr.symbol, sr.avgDelta, sr.minDelta, sr.maxDelta,
			sr.consistency, sr.totalPeriods, rektStr, verdict)
	}

	fmt.Println(strings.Repeat("-", 100))
	fmt.Println()

	// Summary
	fmt.Println("VETO (removal consistently helps - these stocks hurt the strategy):")
	if len(vetoList) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, s := range vetoList {
			fmt.Printf("  %s\n", s)
		}
	}
	fmt.Println()

	fmt.Println("STRONG KEEP (removal consistently hurts - these stocks help the strategy):")
	if len(keepList) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, s := range keepList {
			fmt.Printf("  %s\n", s)
		}
	}
	fmt.Println()

	fmt.Println("NEUTRAL (mixed results across periods):")
	fmt.Printf("  %d stocks\n", len(neutralList))
	fmt.Println()

	// Period breakdown for verbose
	if *flagVerbose {
		fmt.Println(strings.Repeat("=", 90))
		fmt.Println("DETAILED PERIOD BREAKDOWN")
		fmt.Println(strings.Repeat("=", 90))
		for _, sr := range results {
			fmt.Printf("\n%s (avg: %+.2f%%):\n", sr.symbol, sr.avgDelta)
			// Sort periods chronologically
			sort.Slice(sr.periods, func(i, j int) bool {
				return sr.periods[i].period < sr.periods[j].period
			})
			for _, pr := range sr.periods {
				if pr.err != nil {
					fmt.Printf("  %s: ERROR\n", pr.period)
				} else {
					fmt.Printf("  %s: %+.2f%% (baseline: %.2f%%, without: %.2f%%)\n",
						pr.period, pr.deltaPct, pr.baselinePct, pr.annualizedPct)
				}
			}
		}
	}
}

type period struct {
	label string
	start string
	end   string
}

func generatePeriods(startYear int) []period {
	var periods []period
	now := time.Now()

	for year := startYear; year <= now.Year(); year++ {
		// H1: Jan 1 - Jun 30
		h1Start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		h1End := time.Date(year, 7, 1, 0, 0, 0, 0, time.UTC)
		if h1End.Before(now) {
			periods = append(periods, period{
				label: fmt.Sprintf("%dH1", year),
				start: h1Start.Format("2006-01-02"),
				end:   h1End.Format("2006-01-02"),
			})
		}

		// H2: Jul 1 - Dec 31
		h2Start := time.Date(year, 7, 1, 0, 0, 0, 0, time.UTC)
		h2End := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC)
		if h2Start.Before(now) {
			if h2End.After(now) {
				h2End = now
			}
			periods = append(periods, period{
				label: fmt.Sprintf("%dH2", year),
				start: h2Start.Format("2006-01-02"),
				end:   h2End.Format("2006-01-02"),
			})
		}
	}

	return periods
}

func loadStandardPicks() ([]string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("could not find project root (no go.mod)")
		}
		dir = parent
	}

	picksPath := filepath.Join(dir, "etc/picks", *flagPicks)
	f, err := os.Open(picksPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var picks []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			picks = append(picks, line)
		}
	}
	return picks, scanner.Err()
}

func buildVaru() (string, error) {
	tmpFile, err := os.CreateTemp("", "varu-*")
	if err != nil {
		return "", err
	}
	tmpFile.Close()
	binaryPath := tmpFile.Name()

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/varu")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(binaryPath)
		return "", fmt.Errorf("go build failed: %w", err)
	}

	return binaryPath, nil
}

var annualizedRegex = regexp.MustCompile(`([-\d.]+)% annualized`)

type backtestResult struct {
	annualizedPct float64
	liquidated    bool
	err           error
	output        string
}

func runBacktest(binary string, symbols []string, startDate, endDate string) backtestResult {
	var r backtestResult

	args := []string{
		"-backtest",
		"-start", startDate,
		"-end", endDate,
		"-cash", *flagCash,
		"-lookback", *flagLookback,
		"-symbol", strings.Join(symbols, " "),
	}
	if *flagTrail != "" {
		args = append(args, "-trail", *flagTrail)
	}
	if *flagMingap != "" {
		args = append(args, "-mingap", *flagMingap)
	}
	if *flagRekt != "" {
		args = append(args, "-rekt", *flagRekt)
	}

	cmd := exec.Command(binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	r.output = stdout.String() + stderr.String()

	if err != nil {
		// Check if it's an exit code 1 (liquidation)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			r.liquidated = true
			r.annualizedPct = -100 // treat as total loss
			return r
		}
		r.err = fmt.Errorf("backtest failed: %w", err)
		return r
	}

	matches := annualizedRegex.FindStringSubmatch(r.output)
	if len(matches) < 2 {
		r.err = fmt.Errorf("could not parse annualized return from output")
		return r
	}

	annualized, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		r.err = fmt.Errorf("could not parse annualized value: %w", err)
		return r
	}

	r.annualizedPct = annualized
	return r
}
