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
)

var (
	flagSymbol   = flag.String("symbol", "", "space-separated list of candidate symbols to screen")
	flagStart    = flag.String("start", "2025-01-01", "backtest start date")
	flagEnd      = flag.String("end", "", "backtest end date (optional)")
	flagCash     = flag.String("cash", "161844", "starting cash")
	flagTrail    = flag.String("trail", "", "trail stop percentage (optional)")
	flagMingap   = flag.String("mingap", "", "minimum gap percentage (optional)")
	flagLookback = flag.String("lookback", "10", "lookback period in minutes")
	flagWorkers  = flag.Int("workers", 0, "number of parallel workers (default: CPU/2)")
	flagVerbose  = flag.Bool("v", false, "verbose output")
)

type result struct {
	symbol        string
	annualizedPct float64
	baselinePct   float64
	deltaPct      float64
	err           error
	output        string
}

func main() {
	flag.Parse()

	if *flagSymbol == "" {
		log.Fatal("usage: screen -symbol 'SYM1 SYM2 ...' [-start DATE] [-end DATE] [-cash N] [-trail N] [-mingap N]")
	}

	// Parse candidate symbols
	candidates := strings.Fields(*flagSymbol)
	if len(candidates) == 0 {
		log.Fatal("no candidate symbols provided")
	}

	// Load standard varu picks
	standardPicks, err := loadStandardPicks()
	if err != nil {
		log.Fatalf("failed to load standard picks: %v", err)
	}
	log.Printf("Loaded %d standard picks from etc/picks/varu", len(standardPicks))

	// Create set of standard picks for deduplication
	standardSet := make(map[string]bool)
	for _, s := range standardPicks {
		standardSet[s] = true
	}

	// Filter out candidates that are already in standard list
	var filteredCandidates []string
	for _, c := range candidates {
		if !standardSet[c] {
			filteredCandidates = append(filteredCandidates, c)
		} else if *flagVerbose {
			log.Printf("Skipping %s (already in standard list)", c)
		}
	}
	candidates = filteredCandidates

	if len(candidates) == 0 {
		log.Fatal("all candidates are already in the standard list")
	}
	log.Printf("Screening %d candidate symbols", len(candidates))

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

	// First, run baseline (standard picks only)
	log.Printf("Running baseline with standard picks only...")
	baselineResult := runBacktest(varuBinary, standardPicks, "BASELINE")
	if baselineResult.err != nil {
		log.Fatalf("baseline backtest failed: %v\n%s", baselineResult.err, baselineResult.output)
	}
	baselinePct := baselineResult.annualizedPct
	log.Printf("Baseline annualized return: %.2f%%", baselinePct)

	// Run backtests in parallel
	jobs := make(chan string, len(candidates))
	results := make(chan result, len(candidates))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Go(func() {
			for symbol := range jobs {
				// Create symbol list: standard + candidate
				symbols := append([]string{}, standardPicks...)
				symbols = append(symbols, symbol)
				r := runBacktest(varuBinary, symbols, symbol)
				r.baselinePct = baselinePct
				r.deltaPct = r.annualizedPct - baselinePct
				results <- r
			}
		})
	}

	// Send jobs
	for _, c := range candidates {
		jobs <- c
	}
	close(jobs)

	// Wait for completion in separate goroutine
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var allResults []result
	completed := 0
	for r := range results {
		completed++
		allResults = append(allResults, r)
		if r.err != nil {
			log.Printf("[%d/%d] %s: ERROR: %v", completed, len(candidates), r.symbol, r.err)
		} else if *flagVerbose {
			sign := "+"
			if r.deltaPct < 0 {
				sign = ""
			}
			log.Printf("[%d/%d] %s: %.2f%% (%s%.2f%%)", completed, len(candidates), r.symbol, r.annualizedPct, sign, r.deltaPct)
		} else {
			log.Printf("[%d/%d] %s", completed, len(candidates), r.symbol)
		}
	}

	// Sort by delta (best improvement first)
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].deltaPct > allResults[j].deltaPct
	})

	// Print report
	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Printf("SCREENING REPORT (baseline: %.2f%%)\n", baselinePct)
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Printf("%-10s %12s %12s %12s\n", "SYMBOL", "ANNUALIZED", "DELTA", "STATUS")
	fmt.Println("-" + strings.Repeat("-", 59))

	var improved, hurt, errored int
	for _, r := range allResults {
		if r.err != nil {
			fmt.Printf("%-10s %12s %12s %12s\n", r.symbol, "ERROR", "-", r.err.Error())
			errored++
			continue
		}
		status := "neutral"
		if r.deltaPct > 1.0 {
			status = "IMPROVED"
			improved++
		} else if r.deltaPct < -1.0 {
			status = "hurt"
			hurt++
		}
		sign := "+"
		if r.deltaPct < 0 {
			sign = ""
		}
		fmt.Printf("%-10s %11.2f%% %11s%% %12s\n", r.symbol, r.annualizedPct, fmt.Sprintf("%s%.2f", sign, r.deltaPct), status)
	}

	fmt.Println("-" + strings.Repeat("-", 59))
	fmt.Printf("Total: %d candidates | %d improved | %d hurt | %d errors\n", len(allResults), improved, hurt, errored)
	fmt.Println()

	// Print symbols that improved returns by more than 1%
	fmt.Println("Symbols that IMPROVED returns (>1%):")
	for _, r := range allResults {
		if r.err == nil && r.deltaPct > 1.0 {
			fmt.Printf("  %s (+%.2f%%)\n", r.symbol, r.deltaPct)
		}
	}
	fmt.Println()

	// Print symbols that hurt returns by more than 1%
	fmt.Println("Symbols that HURT returns (<-1%):")
	for _, r := range allResults {
		if r.err == nil && r.deltaPct < -1.0 {
			fmt.Printf("  %s (%.2f%%)\n", r.symbol, r.deltaPct)
		}
	}
}

func loadStandardPicks() ([]string, error) {
	// Find project root (look for go.mod)
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

	picksPath := filepath.Join(dir, "etc/picks/varu")
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
	// Create temp file for binary
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

// annualizedRegex matches "X% annualized" in varu output
var annualizedRegex = regexp.MustCompile(`([-\d.]+)% annualized`)

func runBacktest(binary string, symbols []string, label string) result {
	r := result{symbol: label}

	// Build command arguments
	args := []string{
		"-backtest",
		"-start", *flagStart,
		"-cash", *flagCash,
		"-lookback", *flagLookback,
		"-symbol", strings.Join(symbols, " "),
	}
	if *flagEnd != "" {
		args = append(args, "-end", *flagEnd)
	}
	if *flagTrail != "" {
		args = append(args, "-trail", *flagTrail)
	}
	if *flagMingap != "" {
		args = append(args, "-mingap", *flagMingap)
	}

	cmd := exec.Command(binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	r.output = stdout.String() + stderr.String()

	if err != nil {
		r.err = fmt.Errorf("backtest failed: %w", err)
		return r
	}

	// Parse annualized return from output
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
