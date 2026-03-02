// findfutures finds actionable futures trades on CME Globex.
//
// This command enumerates all possible trades you can punch into thinkorswim or
// ninjatrader RIGHT NOW, with current prices and margins. It shows:
//
//   - Outright positions (long/short a single contract)
//   - Calendar spreads (long one month, short another)
//
// For each trade, it provides signals to help you decide:
//
//   - For FX: The carry (interest rate differential) you earn by holding
//   - For commodities: The curve state (backwardation/contango) as a directional signal
//   - For spreads: The spread value you can lock in today
//
// Run with -tutorial for detailed examples of how to use this output.
package main

import (
	"bufio"
	"cmp"
	"dropbear/broker/databento"
	"dropbear/broker/tradovate"
	"dropbear/db"
	"dropbear/loggy"
	"dropbear/netty"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	flagSync             = flag.Bool("sync", false, "fetch latest data from Databento")
	flagAsset            = flag.String("asset", "", "filter to specific asset (e.g., 6J, ES, CL)")
	flagType             = flag.String("type", "", "filter by type: currency, index, energy, metal, ag, rate")
	flagSpreads          = flag.Bool("spreads", false, "show calendar spreads instead of outrights")
	flagUSRate           = flag.Float64("us-rate", 4.5, "current US risk-free rate for FX carry calc")
	flagTutorial         = flag.Bool("tutorial", false, "show detailed tutorial with example scenarios")
	flagTradovateDay     = flag.Bool("tradovate-day", false, "use Tradovate day margin rates (filters to Tradovate-supported symbols)")
	flagTradovateInitial = flag.Bool("tradovate-initial", false, "use Tradovate initial/overnight margin rates (filters to Tradovate-supported symbols)")
)

const tutorial = `
FINDFUTURES TRADING TUTORIAL
============================

This tool shows you actionable futures trades with signals to help decide direction.
There are two modes: outrights (default) and spreads (-spreads).

OUTRIGHT POSITIONS
------------------
An outright is simply going long or short a single futures contract.

Example output (from real data):

  CONTRACT   TYPE          PRICE     MARGIN    CURVE   CARRY% SIGNAL
  ----------------------------------------------------------------------
  CLG6       energy      60.5300      $6500  backwrd  -20.24%   LONG
  ZWH6       ag           515.75      $1650 contango   13.05%  SHORT
  6SH6       currency     1.2740      $4500 contango    3.97%  SHORT
  6MH6       currency    0.05696      $2500  backwrd   -3.90%   LONG

How to read this:

  CLG6 (Crude Oil, Feb 2026):
  - Curve is in backwardation (front months cost more than back)
  - The -20.24% carry means front is 20% HIGHER than back annualized
  - Signal says LONG: backwardation = tight supply = bullish
  - In thinkorswim: Buy /CLG6
  - WARNING: You're exposed to crude price moves! If crude drops $5,
    you lose ~$5,000 per contract regardless of curve shape.

  ZWH6 (Wheat, March 2026):
  - Curve is in contango (13.05% annualized)
  - Back months cost MORE than front
  - Signal says SHORT: contango = oversupply = bearish
  - In thinkorswim: Sell /ZWH6
  - WARNING: Same caveat - full price exposure

  6SH6 (Swiss Franc, March 2026):
  - Curve is in contango (3.97%)
  - This means US rates > Swiss rates by about 3.97%
  - Signal says SHORT: you EARN 3.97% annualized by being short
  - In thinkorswim: Sell /6SH6
  - The futures price decays toward spot, you pocket the difference
  - Risk: CHF strengthens against USD (flight to safety)

  6MH6 (Mexican Peso, March 2026):
  - Curve is in backwardation (-3.90%)
  - This means Mexican rates > US rates
  - Signal says LONG: you EARN the rate differential by being long
  - In thinkorswim: Buy /6MH6
  - Risk: Peso weakens against USD

KEY INSIGHT FOR COMMODITIES:
The curve state (backwardation/contango) is a SIGNAL about supply/demand, not a
return you automatically collect. You profit from price movement in the indicated
direction. If crude drops $5 while you're long, you lose ~$5,000 per contract
regardless of what the curve looks like.

KEY INSIGHT FOR FX:
The carry IS a return you collect over time. If you're short 6S at 3.97% carry,
you're earning the US-Swiss interest rate differential. The futures price
gradually converges toward spot, and you pocket the difference. You still have
FX exposure (CHF strengthening hurts you), but the carry is real and accrues
daily through mark-to-market.

CALENDAR SPREADS (-spreads)
---------------------------
A calendar spread is simultaneously long one month and short another. This
ISOLATES the curve shape from outright price movement.

Example output (from real data):

  SPREAD             TYPE          PRICE     MARGIN   CARRY%     EDGE
  ----------------------------------------------------------------------
  CLG6-CLH6          energy      0.94000       $975  -20.24%  backwrd
  ZWH6-ZWK6          ag         -11.2500       $412   13.05% contango
  ZCH6-ZCK6          ag          -8.2500       $244   11.64% contango
  GCG6-GCJ6          metal      -36.1000      $2488    4.50% contango

How to read this:

  CLG6-CLH6 (Crude Feb-Mar spread):
  - Current spread is $0.94 (Feb is $0.94 HIGHER than Mar)
  - Margin is only $975 (vs $6,500 for outright!)
  - "backwrd" edge means backwardation - front premium
  - In thinkorswim: BUY the /CLG6-/CLH6 calendar spread
  - You profit if backwardation STEEPENS (spread widens to say $1.50)
    Profit = ($1.50 - $0.94) * 1000 = $560
  - You lose if it FLATTENS (spread narrows to say $0.50)
    Loss = ($0.50 - $0.94) * 1000 = -$440

  ZWH6-ZWK6 (Wheat Mar-May spread):
  - Current spread is -$11.25 (Mar is $11.25 BELOW May)
  - "contango" edge
  - In thinkorswim: SELL the /ZWH6-/ZWK6 spread
  - You profit if contango NARROWS (spread goes from -11.25 toward 0)
  - You lose if contango STEEPENS (spread goes to -15)

WHY USE SPREADS?
- Much lower margin (often 1/5th to 1/10th of outright)
- Isolated from outright price moves (crude can drop $10 and your spread barely moves)
- You're trading the CURVE SHAPE, not the commodity price
- Professionals use spreads to express views on supply/demand dynamics

TRADING DECISIONS FRAMEWORK
---------------------------

1. CURRENCY FUTURES (carry is king):
   - Positive carry (contango) = go SHORT (you earn the rate differential)
   - Negative carry (backwardation) = go LONG (you earn the rate differential)
   - The carry IS your expected return (plus/minus spot FX movement)
   - Example: 6S at 3.97% contango, short it, earn ~3.97% annualized

2. COMMODITY OUTRIGHTS (curve as signal):
   - Backwardation (negative carry) = LONG signal (tight supply, bullish)
   - Contango (positive carry) = SHORT signal (oversupply, bearish)
   - But YOU'RE EXPOSED TO PRICE MOVES - the curve is a hint, not a guarantee
   - Use this alongside your own chart analysis

3. COMMODITY SPREADS (pure curve trading):
   - Backwardation: BUY the spread to profit if it steepens
   - Contango: SELL the spread to profit if it narrows
   - Lower risk than outrights, but lower absolute profit potential
   - Great for expressing a view on supply/demand without price direction risk

PRACTICAL EXAMPLES
------------------

Scenario 1: You see CL in steep backwardation (-20% carry)
  - This suggests tight supply, inventory draws
  - Outright play: Long /CLG6 at $60.53, $6,500 margin
    Risk: Crude drops to $54, you lose $6,000
  - Spread play: Buy /CLG6-/CLH6 at $0.94, $975 margin
    If backwardation steepens to $1.50: profit $560
    If it flattens to $0.50: loss $440
    Crude price movement barely affects you!

Scenario 2: You see 6S with 3.97% carry (contango)
  - US rates (~4.5%) higher than Swiss rates (~0.5%)
  - Short /6SH6: you earn ~3.97% annualized as futures price decays
  - Risk: CHF strengthens against dollar (flight to safety, etc.)
  - This is a "carry trade" - you're the bank earning the rate spread

Scenario 3: You see 6M with -3.90% carry (backwardation)
  - Mexican rates (~8.4%) higher than US rates (~4.5%)
  - Long /6MH6: you earn the ~3.9% differential
  - Risk: Peso weakens (EM currency crisis, etc.)
  - High-yield EM carry trade

Scenario 4: Wheat in contango (+13% carry)
  - Oversupply situation, storage costs priced in
  - Outright play: Short /ZWH6 betting prices fall
  - Spread play: Sell /ZWH6-/ZWK6 at -$11.25
    Profit if contango narrows (spread moves toward 0)
    Lower risk than betting on outright wheat price
`

// Trade represents an actionable trade
type Trade struct {
	Symbol         string  // e.g., "CLG6" or "CLG6-CLH6"
	Asset          string  // e.g., "CL"
	TradeType      string  // "outright" or "spread"
	AssetType      string  // "currency", "energy", "metal", "ag", "index", "rate"
	Direction      string  // "LONG" or "SHORT"
	Price          float64 // current price or spread value
	Margin         float64 // required margin
	Notional       float64 // contract notional value
	Leverage       float64 // notional / margin
	Carry          float64 // annualized carry (positive = earn, negative = pay)
	ReturnOnMargin float64 // annualized return on margin (positive = earn, negative = pay)
	CurveState     string  // "contango", "backwrd", or "flat"
	DaysToExp      int     // days to front month expiration
}

// FuturesContract represents a single futures contract
type FuturesContract struct {
	Symbol          string
	Asset           string
	Expiration      time.Time
	SettlementPrice float64
	OpenInterest    int64
	Volume          uint64
}

// ContractSpec holds specifications for a futures contract loaded from CSV
type ContractSpec struct {
	Asset       string  // CME product code (e.g., 6J, ES, CL)
	Type        string  // asset class (currency, index, energy, metal, ag, rate, vol, crypto)
	Margin      float64 // maintenance margin in USD
	Multiplier  float64 // contract multiplier for notional calculation
	SpreadRatio float64 // calendar spread margin as fraction of outright
	Months      string  // which months trade (F=Jan, G=Feb, etc.)
}

// contractSpecs holds all loaded contract specifications
var contractSpecs = make(map[string]ContractSpec)

// loadContractSpecs loads contract specifications from etc/cme/outrights.csv
func loadContractSpecs() error {
	// Find the project root by looking for etc/cme/outrights.csv
	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	csvPath := filepath.Join(projectRoot, "etc", "cme", "outrights.csv")

	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", csvPath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip header
		if strings.HasPrefix(line, "asset,") {
			continue
		}

		fields := strings.Split(line, ",")
		if len(fields) < 6 {
			continue
		}

		margin, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			log.Printf("warning: line %d: invalid margin %q", lineNum, fields[2])
			continue
		}

		multiplier, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			log.Printf("warning: line %d: invalid multiplier %q", lineNum, fields[3])
			continue
		}

		spreadRatio, err := strconv.ParseFloat(fields[4], 64)
		if err != nil {
			spreadRatio = 0.20 // default
		}

		contractSpecs[fields[0]] = ContractSpec{
			Asset:       fields[0],
			Type:        fields[1],
			Margin:      margin,
			Multiplier:  multiplier,
			SpreadRatio: spreadRatio,
			Months:      fields[5],
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", csvPath, err)
	}

	if len(contractSpecs) == 0 {
		return fmt.Errorf("no contracts loaded from %s", csvPath)
	}

	return nil
}

// getSpec returns the contract specification for an asset
func getSpec(asset string) ContractSpec {
	return contractSpecs[asset]
}

// getMargin returns the margin for an asset, using Tradovate rates if flags are set
func getMargin(asset string) (float64, bool) {
	if *flagTradovateDay || *flagTradovateInitial {
		rate, ok := tradovate.MarginRates[asset]
		if !ok {
			return 0, false // not supported by Tradovate
		}
		if *flagTradovateDay {
			return rate.Day.Float64(), true
		}
		return rate.Initial.Float64(), true
	}
	spec := getSpec(asset)
	if spec.Asset == "" {
		return 0, false
	}
	return spec.Margin, true
}

func main() {
	flag.Parse()
	loggy.Init()

	if *flagTutorial {
		fmt.Print(tutorial)
		return
	}

	// Load contract specifications from CSV
	if err := loadContractSpecs(); err != nil {
		loggy.Fatalf("load contract specs: %v", err)
	}

	if *flagSync {
		if err := syncData(); err != nil {
			loggy.Fatalf("sync failed: %v", err)
		}
	}

	contracts, err := loadContracts()
	if err != nil {
		loggy.Fatalf("load contracts: %v", err)
	}

	if len(contracts) == 0 {
		fmt.Println("No contracts found. Run with -sync to fetch data from Databento.")
		os.Exit(1)
	}

	var trades []Trade
	if *flagSpreads {
		trades = generateSpreadTrades(contracts)
	} else {
		trades = generateOutrightTrades(contracts)
	}

	// Filter
	var filtered []Trade
	for _, t := range trades {
		if *flagAsset != "" && t.Asset != *flagAsset {
			continue
		}
		if *flagType != "" && t.AssetType != *flagType {
			continue
		}
		filtered = append(filtered, t)
	}

	// Sort by return on margin descending (best opportunities first)
	slices.SortFunc(filtered, func(a, b Trade) int {
		return cmp.Compare(b.ReturnOnMargin, a.ReturnOnMargin)
	})

	// Print
	if *flagSpreads {
		printSpreads(filtered)
	} else {
		printOutrights(filtered)
	}
}

func generateOutrightTrades(contracts []FuturesContract) []Trade {
	// Group by asset, find front month
	byAsset := make(map[string][]FuturesContract)
	for _, c := range contracts {
		if c.SettlementPrice > 0 {
			byAsset[c.Asset] = append(byAsset[c.Asset], c)
		}
	}

	var trades []Trade
	now := time.Now()

	for asset, cs := range byAsset {
		if len(cs) < 2 {
			continue // Need at least 2 months to determine curve
		}

		// Sort by expiration
		slices.SortFunc(cs, func(a, b FuturesContract) int {
			return a.Expiration.Compare(b.Expiration)
		})

		front := cs[0]
		back := cs[1]

		// Calculate carry from front to back
		daysBetween := int(back.Expiration.Sub(front.Expiration).Hours() / 24)
		if daysBetween <= 0 {
			continue
		}

		priceDiff := back.SettlementPrice - front.SettlementPrice
		carry := (priceDiff / front.SettlementPrice) * (365.0 / float64(daysBetween))

		curveState := "flat"
		if carry > 0.01 {
			curveState = "contango"
		} else if carry < -0.01 {
			curveState = "backwrd"
		}

		spec := getSpec(asset)
		if spec.Asset == "" {
			continue // Skip assets not in our specs
		}
		margin, ok := getMargin(asset)
		if !ok {
			continue // Skip assets not supported (e.g., not on Tradovate when flag set)
		}
		multiplier := spec.Multiplier
		notional := front.SettlementPrice * multiplier
		daysToExp := int(front.Expiration.Sub(now).Hours() / 24)

		// Calculate return on margin (leverage factor)
		leverage := 0.0
		if margin > 0 && notional > 0 {
			leverage = notional / margin
		}

		// For SHORT: you earn carry in contango (positive carry), pay in backwardation
		// For LONG: you earn carry in backwardation (negative carry), pay in contango
		shortCarry := carry // positive = earn, negative = pay
		longCarry := -carry // opposite of short
		shortRoM := shortCarry * leverage
		longRoM := longCarry * leverage

		// Add SHORT entry
		trades = append(trades, Trade{
			Symbol:         front.Symbol,
			Asset:          asset,
			TradeType:      "outright",
			AssetType:      spec.Type,
			Direction:      "SHORT",
			Price:          front.SettlementPrice,
			Margin:         margin,
			Notional:       notional,
			Leverage:       leverage,
			Carry:          shortCarry,
			ReturnOnMargin: shortRoM,
			CurveState:     curveState,
			DaysToExp:      daysToExp,
		})

		// Add LONG entry
		trades = append(trades, Trade{
			Symbol:         front.Symbol,
			Asset:          asset,
			TradeType:      "outright",
			AssetType:      spec.Type,
			Direction:      "LONG",
			Price:          front.SettlementPrice,
			Margin:         margin,
			Notional:       notional,
			Leverage:       leverage,
			Carry:          longCarry,
			ReturnOnMargin: longRoM,
			CurveState:     curveState,
			DaysToExp:      daysToExp,
		})
	}

	return trades
}

func generateSpreadTrades(contracts []FuturesContract) []Trade {
	// Group by asset
	byAsset := make(map[string][]FuturesContract)
	for _, c := range contracts {
		if c.SettlementPrice > 0 {
			byAsset[c.Asset] = append(byAsset[c.Asset], c)
		}
	}

	var trades []Trade
	now := time.Now()

	for asset, cs := range byAsset {
		if len(cs) < 2 {
			continue
		}

		// Sort by expiration
		slices.SortFunc(cs, func(a, b FuturesContract) int {
			return a.Expiration.Compare(b.Expiration)
		})

		spec := getSpec(asset)
		if spec.Asset == "" {
			continue // Skip assets not in our specs
		}
		outrightMargin, ok := getMargin(asset)
		if !ok {
			continue // Skip assets not supported (e.g., not on Tradovate when flag set)
		}
		spreadRatio := spec.SpreadRatio
		if spreadRatio == 0 {
			spreadRatio = 0.20 // default 20%
		}

		// Generate adjacent month spreads
		for i := 0; i < len(cs)-1; i++ {
			front := cs[i]
			back := cs[i+1]

			spreadValue := front.SettlementPrice - back.SettlementPrice
			daysBetween := int(back.Expiration.Sub(front.Expiration).Hours() / 24)
			if daysBetween <= 0 {
				continue
			}

			// Carry for this spread
			carry := (spreadValue / front.SettlementPrice) * (365.0 / float64(daysBetween)) * -1

			curveState := "flat"
			if spreadValue > 0 {
				curveState = "backwrd" // front > back
			} else if spreadValue < 0 {
				curveState = "contango" // front < back
			}

			spreadMargin := outrightMargin * spreadRatio
			multiplier := spec.Multiplier
			// For spreads, the "notional" exposure is the spread value * multiplier
			spreadNotional := math.Abs(spreadValue) * multiplier
			daysToExp := int(front.Expiration.Sub(now).Hours() / 24)

			// Return on margin for spread
			// This represents how much the spread would need to move (as % of notional)
			// to generate returns on your margin
			returnOnMargin := 0.0
			if spreadMargin > 0 && multiplier > 0 {
				// For spreads, we show potential return if spread moves by carry amount
				absCarry := carry
				if absCarry < 0 {
					absCarry = -absCarry
				}
				// Use front price notional for consistency
				frontNotional := front.SettlementPrice * multiplier
				returnOnMargin = absCarry * (frontNotional / spreadMargin)
			}

			trades = append(trades, Trade{
				Symbol:         fmt.Sprintf("%s-%s", front.Symbol, back.Symbol),
				Asset:          asset,
				TradeType:      "spread",
				AssetType:      spec.Type,
				Direction:      "BUY",
				Price:          spreadValue,
				Margin:         spreadMargin,
				Notional:       spreadNotional,
				Carry:          carry,
				ReturnOnMargin: returnOnMargin,
				CurveState:     curveState,
				DaysToExp:      daysToExp,
			})
		}
	}

	return trades
}

func printOutrights(trades []Trade) {
	// Header
	fmt.Printf("%-10s %-5s %-8s %10s %7s %8s %8s %8s %8s %8s %8s\n",
		"CONTRACT", "DIR", "TYPE", "MARGIN", "CARRY", "RIGHT1%", "RIGHT2%", "RIGHT5%", "RIGHT10%", "RIGHT20%", "RIGHT40%")
	fmt.Println(strings.Repeat("-", 104))

	for _, t := range trades {
		marginStr := "-"
		if t.Margin > 0 {
			marginStr = fmt.Sprintf("$%.0f", t.Margin)
		}

		// Calculate net return on margin for different directional moves
		// "RIGHT X%" means your directional bet was correct by X%
		// LONG right = price UP, SHORT right = price DOWN
		right1 := t.Carry + 0.01*t.Leverage
		right2 := t.Carry + 0.02*t.Leverage
		right5 := t.Carry + 0.05*t.Leverage
		right10 := t.Carry + 0.10*t.Leverage
		right20 := t.Carry + 0.20*t.Leverage
		right40 := t.Carry + 0.40*t.Leverage

		fmt.Printf("%-10s %-5s %-8s %10s %+6.0f%% %+7.0f%% %+7.0f%% %+7.0f%% %+7.0f%% %+7.0f%% %+7.0f%%\n",
			t.Symbol,
			t.Direction,
			t.AssetType,
			marginStr,
			t.ReturnOnMargin*100,
			right1*100,
			right2*100,
			right5*100,
			right10*100,
			right20*100,
			right40*100)
	}

	if len(trades) == 0 {
		fmt.Println("No trades found matching criteria.")
	}
}

func printSpreads(trades []Trade) {
	fmt.Printf("%-18s %-8s %12s %10s %8s %10s %8s\n",
		"SPREAD", "TYPE", "PRICE", "MARGIN", "CARRY%", "RET/MARGIN", "EDGE")
	fmt.Println(strings.Repeat("-", 82))

	for _, t := range trades {
		marginStr := "-"
		if t.Margin > 0 {
			marginStr = fmt.Sprintf("$%.0f", t.Margin)
		}

		priceStr := formatPrice(t.Price)

		retMarginStr := "-"
		if t.ReturnOnMargin > 0 {
			retMarginStr = fmt.Sprintf("%.0f%%", t.ReturnOnMargin*100)
		}

		fmt.Printf("%-18s %-8s %12s %10s %7.2f%% %10s %8s\n",
			t.Symbol,
			t.AssetType,
			priceStr,
			marginStr,
			t.Carry*100,
			retMarginStr,
			t.CurveState)
	}

	if len(trades) == 0 {
		fmt.Println("No spreads found matching criteria.")
	} else {
		fmt.Printf("\n%d spreads shown.\n", len(trades))
		fmt.Println("Use -tutorial for help interpreting this output.")
	}
}

func formatPrice(price float64) string {
	absPrice := math.Abs(price)
	if absPrice >= 100 {
		return fmt.Sprintf("%.2f", price)
	} else if absPrice >= 1 {
		return fmt.Sprintf("%.4f", price)
	} else if absPrice >= 0.01 {
		return fmt.Sprintf("%.5f", price)
	}
	return fmt.Sprintf("%.6f", price)
}

// ============================================================================
// Data fetching (unchanged from original)
// ============================================================================

func syncData() error {
	conn := db.Get()
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS futures_contracts (
			symbol TEXT PRIMARY KEY,
			asset TEXT NOT NULL,
			expiration INTEGER,
			settlement_price REAL,
			open_interest INTEGER,
			volume INTEGER,
			updated_at INTEGER
		)
	`)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}
	_, _ = conn.Exec(`CREATE INDEX IF NOT EXISTS idx_futures_asset ON futures_contracts(asset)`)

	symbols := generateSymbols()

	// Filter symbols if -asset specified
	if *flagAsset != "" {
		var filtered []string
		for _, sym := range symbols {
			asset, _ := parseSymbol(sym)
			if asset == *flagAsset {
				filtered = append(filtered, sym)
			}
		}
		symbols = filtered
	}

	log.Printf("fetching %d symbols with 50 workers...", len(symbols))

	yesterday := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	// Fetch all symbols in parallel with 50 workers
	type symbolResult struct {
		symbol string
		data   symbolData
		err    error
	}

	resultsChan := make(chan symbolResult, len(symbols))
	symbolChan := make(chan string, len(symbols))

	// Start 50 workers
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sym := range symbolChan {
				data, err := fetchSymbolPrice(sym, yesterday, today)
				resultsChan <- symbolResult{symbol: sym, data: data, err: err}
			}
		}()
	}

	// Send symbols to workers
	for _, sym := range symbols {
		symbolChan <- sym
	}
	close(symbolChan)

	// Wait for workers and close results channel
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	allResults := make(map[string]symbolData)
	fetchErrors := 0
	for sr := range resultsChan {
		if sr.err != nil {
			fetchErrors++
			continue
		}
		if sr.data.Price > 0 {
			allResults[sr.symbol] = sr.data
		}
	}

	log.Printf("fetched %d contracts (%d errors)", len(allResults), fetchErrors)

	// Store in database
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO futures_contracts
		(symbol, asset, expiration, settlement_price, open_interest, volume, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	stored := 0

	for sym, data := range allResults {
		if data.Price <= 0 {
			continue
		}
		asset, expiry := parseSymbol(sym)
		if asset == "" {
			continue
		}
		_, err = stmt.Exec(sym, asset, expiry.UnixNano(), data.Price, 0, data.Volume, now)
		if err != nil {
			log.Printf("error storing %s: %v", sym, err)
			continue
		}
		stored++
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("stored %d contracts", stored)
	return nil
}

type symbolData struct {
	Price  float64
	Volume uint64
}

func fetchSymbolPrice(symbol, startDate, endDate string) (symbolData, error) {
	url := fmt.Sprintf(
		"https://hist.databento.com/v0/timeseries.get_range?dataset=GLBX.MDP3&symbols=%s&schema=ohlcv-1d&start=%sT00:00:00&end=%sT00:00:00&encoding=json",
		symbol, startDate, endDate,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("GET %s -> error: %v", url, err)
		return symbolData{}, err
	}
	req.SetBasicAuth(string(databento.MustLoadDefaultKey()), "")

	resp, err := netty.BulkHttpClient.Do(req)
	if err != nil {
		log.Printf("GET %s -> error: %v", url, err)
		return symbolData{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("GET %s -> 404", url)
		return symbolData{}, nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		log.Printf("GET %s -> HTTP %d", url, resp.StatusCode)
		return symbolData{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := parseOHLCVJSON(resp.Body)
	if err != nil {
		log.Printf("GET %s -> parse error: %v", url, err)
		return data, err
	}
	if data.Price > 0 {
		log.Printf("GET %s -> %.4f", url, data.Price)
	} else {
		log.Printf("GET %s -> no data", url)
	}
	return data, nil
}

type ohlcvRecord struct {
	Close  string `json:"close"`
	Volume string `json:"volume"`
}

func parseOHLCVJSON(r io.Reader) (symbolData, error) {
	decoder := json.NewDecoder(r)
	var last symbolData

	for {
		var record ohlcvRecord
		if err := decoder.Decode(&record); err == io.EOF {
			break
		} else if err != nil {
			return last, fmt.Errorf("decode JSON: %w", err)
		}

		if record.Close != "" {
			// Databento JSON uses fixed-point prices as strings scaled by 1e9
			var close float64
			var volume uint64
			fmt.Sscanf(record.Close, "%f", &close)
			fmt.Sscanf(record.Volume, "%d", &volume)
			last = symbolData{Price: close / 1e9, Volume: volume}
		}
	}

	return last, nil
}

func generateSymbols() []string {
	now := time.Now()
	currentYear := now.Year()
	currentMonth := int(now.Month())

	var symbols []string
	for _, spec := range contractSpecs {
		// Only generate next 6 valid contract months per asset
		count := 0
		for yearOffset := 0; yearOffset < 2 && count < 6; yearOffset++ {
			year := currentYear + yearOffset
			for _, monthCode := range spec.Months {
				month := monthCodes[byte(monthCode)]
				// Skip if this month already passed (contracts expire mid-month)
				if year == currentYear && month <= currentMonth {
					continue
				}
				sym := fmt.Sprintf("%s%c%d", spec.Asset, monthCode, year%10)
				symbols = append(symbols, sym)
				count++
				if count >= 6 {
					break
				}
			}
		}
	}

	return symbols
}

var monthCodes = map[byte]int{
	'F': 1, 'G': 2, 'H': 3, 'J': 4, 'K': 5, 'M': 6,
	'N': 7, 'Q': 8, 'U': 9, 'V': 10, 'X': 11, 'Z': 12,
}

var symbolRegex = regexp.MustCompile(`^([A-Z0-9]+)([FGHJKMNQUVXZ])(\d)$`)

func parseSymbol(symbol string) (asset string, expiry time.Time) {
	symbol = strings.TrimSpace(symbol)

	matches := symbolRegex.FindStringSubmatch(symbol)
	if matches == nil {
		return "", time.Time{}
	}

	asset = matches[1]
	monthCode := matches[2][0]
	yearDigit := int(matches[3][0] - '0')

	month := monthCodes[monthCode]
	if month == 0 {
		return "", time.Time{}
	}

	currentYear := time.Now().Year()
	currentDecade := (currentYear / 10) * 10
	year := currentDecade + yearDigit

	if year < currentYear-5 {
		year += 10
	}

	expiry = time.Date(year, time.Month(month), 15, 0, 0, 0, 0, time.UTC)
	return asset, expiry
}

func loadContracts() ([]FuturesContract, error) {
	conn := db.Get()

	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='futures_contracts'").Scan(&count)
	if err != nil || count == 0 {
		return nil, nil
	}

	rows, err := conn.Query(`
		SELECT symbol, asset, expiration, settlement_price, open_interest, volume
		FROM futures_contracts
		WHERE expiration > ?
		ORDER BY asset, expiration
	`, time.Now().UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contracts []FuturesContract
	for rows.Next() {
		var c FuturesContract
		var expNanos int64
		err := rows.Scan(&c.Symbol, &c.Asset, &expNanos, &c.SettlementPrice, &c.OpenInterest, &c.Volume)
		if err != nil {
			log.Printf("error scanning row: %v", err)
			continue
		}
		c.Expiration = time.Unix(0, expNanos)
		contracts = append(contracts, c)
	}

	return contracts, nil
}
