// findfutures finds futures carry trade opportunities.
//
// This command analyzes CME futures to find contracts with attractive carry (roll yield).
// The classic example is shorting Japanese Yen futures (/6J) which historically provided
// ~61% annualized positive carry due to interest rate differentials between US and Japan.
//
// Usage:
//
//	go run ./cmd/findfutures                    # analyze and print opportunities
//	go run ./cmd/findfutures -sync              # fetch latest data from Databento first
//	go run ./cmd/findfutures -asset 6J          # filter to specific asset
//	go run ./cmd/findfutures -type currency     # filter to currency futures only
//
// The carry is calculated as:
//
//	Annualized Carry = (Back - Front) / Front * (365 / Days Between)
//
// For currency futures, this approximates the interest rate differential between
// USD and the foreign currency under covered interest parity.
package main

import (
	"bytes"
	"cmp"
	"dropbear/broker/databento"
	"dropbear/db"
	"dropbear/loggy"
	"dropbear/netty"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
	"unsafe"
)

func init() {
	netty.SetOffline()
}

var (
	flagSync   = flag.Bool("sync", false, "fetch latest data from Databento")
	flagAsset  = flag.String("asset", "", "filter to specific asset (e.g., 6J, ES, NQ)")
	flagType   = flag.String("type", "", "filter by type: currency, index, commodity, rate")
	flagShort  = flag.Bool("short", true, "show short opportunities (contango)")
	flagLong   = flag.Bool("long", false, "show long opportunities (backwardation)")
	flagMinOI  = flag.Int("min-oi", 0, "minimum open interest (default 0, set higher to filter)")
	flagUSRate = flag.Float64("us-rate", 4.5, "current US risk-free rate (for implied foreign rate calc)")
	flagSimple = flag.Bool("simple", false, "show simple output: one contract per asset to trade")
)

// FuturesContract represents a single futures contract
type FuturesContract struct {
	Symbol          string
	Asset           string // product root (e.g., "6J")
	Expiration      time.Time
	SettlementPrice float64
	OpenInterest    int64
	Volume          uint64
}

// CarryOpportunity represents a carry trade analysis
type CarryOpportunity struct {
	Asset             string
	FrontSymbol       string
	BackSymbol        string
	FrontExpiry       time.Time
	BackExpiry        time.Time
	FrontPrice        float64
	BackPrice         float64
	DaysBetween       int
	AnnualizedCarry   float64 // positive = contango (short profitable)
	ImpliedForeignRat float64 // for currency futures
	FrontOI           int64
	BackOI            int64
	AssetType         string  // currency, index, commodity, rate
	Direction         string  // "short" or "long"
	Margin            float64 // maintenance margin in USD
	Notional          float64 // contract notional value in USD
	ReturnOnMargin    float64 // annualized carry as % of margin
}

// CME maintenance margin requirements (USD) - updated Jan 2026
// Source: https://www.cmegroup.com/clearing/margins/outright-vol-scans.html
var marginRequirements = map[string]float64{
	// FX futures
	"6J": 2800,  // Japanese Yen
	"6E": 2700,  // Euro
	"6S": 4500,  // Swiss Franc
	"6A": 1900,  // Australian Dollar
	"6B": 2000,  // British Pound
	"6C": 1000,  // Canadian Dollar
	"6N": 1300,  // New Zealand Dollar
	"6M": 2500,  // Mexican Peso (approx)

	// Equity index futures
	"ES":  22731, // E-mini S&P 500
	"NQ":  33563, // E-mini Nasdaq 100
	"YM":  14245, // E-mini Dow
	"RTY": 9491,  // E-mini Russell 2000

	// Metals
	"GC": 24881, // Gold (100 oz)
	"SI": 45417, // Silver (5000 oz)
	"HG": 10000, // Copper

	// Agriculture
	"ZC": 975,  // Corn
	"ZW": 1650, // Wheat
	"ZS": 2000, // Soybeans

	// Rates
	"ZN": 2000, // 10-Year T-Note (approx)
	"ZB": 3500, // 30-Year T-Bond (approx)
	"ZT": 1000, // 2-Year T-Note (approx)
	"ZF": 1500, // 5-Year T-Note (approx)

	// Energy
	"CL": 6500, // Crude Oil
	"NG": 3000, // Natural Gas
}

// Contract multipliers for calculating notional value
var contractMultipliers = map[string]float64{
	// FX futures (units of foreign currency)
	"6J": 12500000, // 12.5M JPY
	"6E": 125000,   // 125K EUR
	"6S": 125000,   // 125K CHF
	"6A": 100000,   // 100K AUD
	"6B": 62500,    // 62.5K GBP
	"6C": 100000,   // 100K CAD
	"6N": 100000,   // 100K NZD
	"6M": 500000,   // 500K MXN

	// Equity index (multiplier × index value)
	"ES":  50, // $50 × S&P 500
	"NQ":  20, // $20 × Nasdaq 100
	"YM":  5,  // $5 × Dow Jones
	"RTY": 50, // $50 × Russell 2000

	// Metals
	"GC": 100,   // 100 troy oz
	"SI": 5000,  // 5000 troy oz
	"HG": 25000, // 25,000 lbs

	// Agriculture (bushels) - prices quoted in cents, so divide by 100
	"ZC": 50, // 5000 bushels ÷ 100 (price in cents)
	"ZW": 50, // 5000 bushels ÷ 100 (price in cents)
	"ZS": 50, // 5000 bushels ÷ 100 (price in cents)

	// Rates (face value / 1000 for display price)
	"ZN": 1000, // $100K face
	"ZB": 1000, // $100K face
	"ZT": 2000, // $200K face
	"ZF": 1000, // $100K face

	// Energy
	"CL": 1000, // 1000 barrels
	"NG": 10000, // 10,000 MMBtu
}

func main() {
	flag.Parse()
	loggy.Init()

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

	opportunities := analyzeCarry(contracts)

	// Filter and sort
	var filtered []CarryOpportunity
	for _, opp := range opportunities {
		if *flagAsset != "" && opp.Asset != *flagAsset {
			continue
		}
		if *flagType != "" && opp.AssetType != *flagType {
			continue
		}
		if opp.FrontOI < int64(*flagMinOI) || opp.BackOI < int64(*flagMinOI) {
			continue
		}
		if *flagShort && opp.Direction == "short" {
			filtered = append(filtered, opp)
		}
		if *flagLong && opp.Direction == "long" {
			filtered = append(filtered, opp)
		}
	}

	// Sort by absolute annualized carry descending
	slices.SortFunc(filtered, func(a, b CarryOpportunity) int {
		absA := a.AnnualizedCarry
		if absA < 0 {
			absA = -absA
		}
		absB := b.AnnualizedCarry
		if absB < 0 {
			absB = -absB
		}
		return cmp.Compare(absB, absA)
	})

	// Print results
	if *flagSimple {
		// Simple mode: one contract per asset (front month only, with carry from front→next spread)
		// Keep the shortest spread (front→next month) for each asset
		byAsset := make(map[string]CarryOpportunity)
		for _, opp := range filtered {
			existing, ok := byAsset[opp.Asset]
			if !ok || opp.DaysBetween < existing.DaysBetween {
				byAsset[opp.Asset] = opp
			}
		}
		var simple []CarryOpportunity
		for _, opp := range byAsset {
			simple = append(simple, opp)
		}

		// Re-sort by return on margin descending
		slices.SortFunc(simple, func(a, b CarryOpportunity) int {
			return cmp.Compare(b.ReturnOnMargin, a.ReturnOnMargin)
		})

		fmt.Printf("%-10s %-8s %12s %10s %8s %10s %-5s\n",
			"CONTRACT", "TYPE", "PRICE", "MARGIN", "CARRY%", "RET/MARGIN", "ACTION")
		fmt.Println(strings.Repeat("-", 75))

		for _, opp := range simple {
			action := "SHORT"
			if opp.Direction == "long" {
				action = "LONG"
			}
			marginStr := fmt.Sprintf("$%.0f", opp.Margin)
			retOnMargin := ""
			if opp.ReturnOnMargin > 0 {
				retOnMargin = fmt.Sprintf("%8.1f%%", opp.ReturnOnMargin*100)
			} else {
				retOnMargin = "       -"
			}
			fmt.Printf("%-10s %-8s %12.6f %10s %7.2f%% %10s %-5s\n",
				opp.FrontSymbol,
				opp.AssetType[:min(8, len(opp.AssetType))],
				opp.FrontPrice,
				marginStr,
				opp.AnnualizedCarry*100,
				retOnMargin,
				action)
		}

		if len(simple) == 0 {
			fmt.Println("No opportunities found matching criteria.")
		} else {
			fmt.Printf("\n%d contracts to trade\n", len(simple))
		}
	} else {
		// Detailed mode: show all front→back spreads
		fmt.Printf("%-8s %-8s %-12s %-12s %6s %12s %12s %8s %8s %10s %-5s\n",
			"ASSET", "TYPE", "FRONT", "BACK", "DAYS", "FRONT_PX", "BACK_PX", "CARRY%", "IMP_RT%", "RET/MARGIN", "DIR")
		fmt.Println(strings.Repeat("-", 120))

		for _, opp := range filtered {
			impliedRate := ""
			if opp.AssetType == "currency" {
				impliedRate = fmt.Sprintf("%6.2f%%", opp.ImpliedForeignRat*100)
			} else {
				impliedRate = "     -"
			}
			retOnMargin := ""
			if opp.ReturnOnMargin > 0 {
				retOnMargin = fmt.Sprintf("%8.1f%%", opp.ReturnOnMargin*100)
			} else {
				retOnMargin = "       -"
			}
			fmt.Printf("%-8s %-8s %-12s %-12s %6d %12.6f %12.6f %7.2f%% %8s %10s %-5s\n",
				opp.Asset,
				opp.AssetType[:min(8, len(opp.AssetType))],
				opp.FrontSymbol,
				opp.BackSymbol,
				opp.DaysBetween,
				opp.FrontPrice,
				opp.BackPrice,
				opp.AnnualizedCarry*100,
				impliedRate,
				retOnMargin,
				opp.Direction)
		}

		if len(filtered) == 0 {
			fmt.Println("No opportunities found matching criteria.")
		} else {
			fmt.Printf("\n%d opportunities found\n", len(filtered))
		}
	}
}

func syncData() error {
	apiKey, err := databento.GetKey()
	if err != nil {
		return fmt.Errorf("get API key: %w (create ~/.databento.key)", err)
	}

	// Create tables
	conn := db.Get()
	_, err = conn.Exec(`
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

	// Major futures symbols to fetch
	// Each symbol is fetched individually to avoid mapping issues
	symbols := generateSymbols()
	log.Printf("fetching %d CME futures contracts from Databento...", len(symbols))

	yesterday := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

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
	errors := 0

	// Fetch each symbol individually
	client := &http.Client{Timeout: 30 * time.Second}
	rateLimiter := netty.NewTokenBucketPerMinute(100) // Databento rate limit

	for _, sym := range symbols {
		rateLimiter.Get()

		price, volume, err := fetchSymbolPrice(client, apiKey, sym, yesterday, today)
		if err != nil {
			errors++
			continue
		}

		if price <= 0 {
			continue
		}

		asset, expiry := parseSymbol(sym)
		if asset == "" {
			continue
		}

		_, err = stmt.Exec(sym, asset, expiry.UnixNano(), price, 0, volume, now)
		if err != nil {
			log.Printf("error storing %s: %v", sym, err)
			continue
		}
		stored++
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("stored %d contracts (%d errors)", stored, errors)
	return nil
}

func generateSymbols() []string {
	// Generate symbols for major futures
	// Format: ASSET + MONTH_CODE + YEAR_DIGIT
	// Month codes: F=Jan, G=Feb, H=Mar, J=Apr, K=May, M=Jun, N=Jul, Q=Aug, U=Sep, V=Oct, X=Nov, Z=Dec

	assets := []struct {
		name   string
		months string // which months are liquid
	}{
		// Currency futures (quarterly)
		{"6J", "HMUZ"}, // Japanese Yen
		{"6E", "HMUZ"}, // Euro
		{"6B", "HMUZ"}, // British Pound
		{"6A", "HMUZ"}, // Australian Dollar
		{"6C", "HMUZ"}, // Canadian Dollar
		{"6S", "HMUZ"}, // Swiss Franc
		{"6N", "HMUZ"}, // New Zealand Dollar
		{"6M", "HMUZ"}, // Mexican Peso

		// Index futures (quarterly)
		{"ES", "HMUZ"},  // E-mini S&P 500
		{"NQ", "HMUZ"},  // E-mini Nasdaq 100
		{"YM", "HMUZ"},  // E-mini Dow
		{"RTY", "HMUZ"}, // E-mini Russell 2000

		// Treasury futures (quarterly)
		{"ZN", "HMUZ"}, // 10-Year T-Note
		{"ZB", "HMUZ"}, // 30-Year T-Bond
		{"ZF", "HMUZ"}, // 5-Year T-Note
		{"ZT", "HMUZ"}, // 2-Year T-Note

		// Energy (monthly)
		{"CL", "FGHJKMNQUVXZ"}, // Crude Oil
		{"NG", "FGHJKMNQUVXZ"}, // Natural Gas

		// Metals (bi-monthly typically, but varies)
		{"GC", "GJMQVZ"}, // Gold
		{"SI", "HKNUZ"},  // Silver
		{"HG", "HKNUZ"},  // Copper

		// Agriculture (varies)
		{"ZC", "HKNUZ"},   // Corn
		{"ZS", "FHKNQUX"}, // Soybeans
		{"ZW", "HKNUZ"},   // Wheat
	}

	// Generate for current year and next year
	currentYear := time.Now().Year()
	years := []int{currentYear % 10, (currentYear + 1) % 10}

	var symbols []string
	for _, asset := range assets {
		for _, month := range asset.months {
			for _, year := range years {
				sym := fmt.Sprintf("%s%c%d", asset.name, month, year)
				symbols = append(symbols, sym)
			}
		}
	}

	return symbols
}

func fetchSymbolPrice(client *http.Client, apiKey, symbol, startDate, endDate string) (float64, uint64, error) {
	url := fmt.Sprintf(
		"https://hist.databento.com/v0/timeseries.get_range?dataset=GLBX.MDP3&symbols=%s&schema=ohlcv-1d&start=%sT00:00:00&end=%sT00:00:00&encoding=dbn",
		symbol, startDate, endDate,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, 0, err
	}
	req.SetBasicAuth(apiKey, "")

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, 0, nil // Symbol not found/no data
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}

	return parseOHLCVSingle(data)
}

func parseOHLCVSingle(data []byte) (float64, uint64, error) {
	if len(data) < 8 {
		return 0, 0, nil
	}
	if string(data[:3]) != "DBN" {
		return 0, 0, fmt.Errorf("invalid DBN magic")
	}

	metaLen := binary.LittleEndian.Uint32(data[4:8])
	headerSize := 8 + int(metaLen)

	if headerSize >= len(data) {
		return 0, 0, nil // No records
	}

	records := data[headerSize:]
	ohlcvSize := int(unsafe.Sizeof(databento.OhlcvMsg{}))

	// Find the latest OHLCV record
	var lastClose int64
	var lastVolume uint64

	for offset := 0; offset+ohlcvSize <= len(records); offset += ohlcvSize {
		var ohlcv databento.OhlcvMsg
		reader := bytes.NewReader(records[offset : offset+ohlcvSize])
		if err := binary.Read(reader, binary.LittleEndian, &ohlcv); err != nil {
			continue
		}

		if ohlcv.Header.RType == databento.RTypeOhlcv1D {
			lastClose = ohlcv.Close
			lastVolume = ohlcv.Volume
		}
	}

	if lastClose == 0 {
		return 0, 0, nil
	}

	return float64(lastClose) / float64(databento.PriceScale), lastVolume, nil
}

// CME futures month codes
var monthCodes = map[byte]int{
	'F': 1, 'G': 2, 'H': 3, 'J': 4, 'K': 5, 'M': 6,
	'N': 7, 'Q': 8, 'U': 9, 'V': 10, 'X': 11, 'Z': 12,
}

// parseSymbol extracts asset and expiration from CME symbol
// Examples: "6JH5" -> ("6J", March 2025), "ESZ4" -> ("ES", December 2024)
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

	// Determine full year
	currentYear := time.Now().Year()
	currentDecade := (currentYear / 10) * 10
	year := currentDecade + yearDigit

	// If the year is more than 5 years in the past, it's probably next decade
	if year < currentYear-5 {
		year += 10
	}

	// Expiration is typically third Friday of the month
	// Use 15th as approximation
	expiry = time.Date(year, time.Month(month), 15, 0, 0, 0, 0, time.UTC)
	return asset, expiry
}

func loadContracts() ([]FuturesContract, error) {
	conn := db.Get()

	// Check if table exists
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
			continue
		}
		c.Expiration = time.Unix(0, expNanos)
		contracts = append(contracts, c)
	}

	return contracts, nil
}

func analyzeCarry(contracts []FuturesContract) []CarryOpportunity {
	// Group by asset
	byAsset := make(map[string][]FuturesContract)
	for _, c := range contracts {
		if c.SettlementPrice > 0 {
			byAsset[c.Asset] = append(byAsset[c.Asset], c)
		}
	}

	var opportunities []CarryOpportunity

	for asset, cs := range byAsset {
		if len(cs) < 2 {
			continue
		}

		// Sort by expiration
		slices.SortFunc(cs, func(a, b FuturesContract) int {
			return a.Expiration.Compare(b.Expiration)
		})

		// Analyze front month vs next months
		front := cs[0]
		assetType := classifyAsset(asset)

		for i := 1; i < len(cs) && i <= 3; i++ { // Compare up to 3 back months
			back := cs[i]

			daysBetween := int(back.Expiration.Sub(front.Expiration).Hours() / 24)
			if daysBetween <= 0 {
				continue
			}

			// Calculate annualized carry
			// Positive carry = futures curve in contango (back > front)
			// Shorting in contango profits from roll
			priceDiff := back.SettlementPrice - front.SettlementPrice
			carry := (priceDiff / front.SettlementPrice) * (365.0 / float64(daysBetween))

			direction := "short"
			if carry < 0 {
				direction = "long"
			}

			// For currency futures, calculate implied foreign interest rate
			// Under CIP: F/S = (1 + r_domestic) / (1 + r_foreign)
			// So: r_foreign ≈ r_domestic - carry
			impliedForeignRate := 0.0
			if assetType == "currency" {
				impliedForeignRate = *flagUSRate/100.0 - carry
			}

			// Calculate margin and notional for return on margin
			margin := marginRequirements[asset]
			multiplier := contractMultipliers[asset]
			notional := 0.0
			returnOnMargin := 0.0

			if multiplier > 0 {
				// For FX: notional = price × multiplier (gives USD value)
				// For index: notional = price × multiplier
				// For commodities: notional = price × multiplier
				notional = front.SettlementPrice * multiplier
			}

			if margin > 0 && notional > 0 {
				// Return on margin = carry × (notional / margin)
				// This is the leveraged return
				returnOnMargin = carry * (notional / margin)
				if returnOnMargin < 0 {
					returnOnMargin = -returnOnMargin
				}
			}

			opp := CarryOpportunity{
				Asset:             asset,
				FrontSymbol:       front.Symbol,
				BackSymbol:        back.Symbol,
				FrontExpiry:       front.Expiration,
				BackExpiry:        back.Expiration,
				FrontPrice:        front.SettlementPrice,
				BackPrice:         back.SettlementPrice,
				DaysBetween:       daysBetween,
				AnnualizedCarry:   carry,
				ImpliedForeignRat: impliedForeignRate,
				FrontOI:           front.OpenInterest,
				BackOI:            back.OpenInterest,
				AssetType:         assetType,
				Direction:         direction,
				Margin:            margin,
				Notional:          notional,
				ReturnOnMargin:    returnOnMargin,
			}

			opportunities = append(opportunities, opp)
		}
	}

	return opportunities
}

func classifyAsset(asset string) string {
	// Currency futures typically start with 6
	if strings.HasPrefix(asset, "6") {
		return "currency"
	}

	// Major index futures
	indexAssets := map[string]bool{
		"ES": true, "NQ": true, "YM": true, "RTY": true,
		"NKD": true, "NIY": true, "EMD": true,
		"MES": true, "MNQ": true, "MYM": true, "M2K": true,
	}
	if indexAssets[asset] {
		return "index"
	}

	// Interest rate futures
	rateAssets := map[string]bool{
		"ZQ": true, "ZT": true, "ZF": true, "ZN": true, "ZB": true,
		"GE": true, "SR3": true, "SR1": true, "FF": true,
	}
	if rateAssets[asset] {
		return "rate"
	}

	// Energy futures
	energyAssets := map[string]bool{
		"CL": true, "NG": true, "HO": true, "RB": true, "BZ": true,
		"MCL": true, "MNG": true,
	}
	if energyAssets[asset] {
		return "commodity"
	}

	// Metals
	metalAssets := map[string]bool{
		"GC": true, "SI": true, "HG": true, "PL": true, "PA": true,
		"MGC": true, "SIL": true,
	}
	if metalAssets[asset] {
		return "commodity"
	}

	// Agricultural
	agAssets := map[string]bool{
		"ZC": true, "ZS": true, "ZW": true, "ZM": true, "ZL": true,
		"LE": true, "HE": true, "GF": true,
		"KC": true, "SB": true, "CC": true, "CT": true,
	}
	if agAssets[asset] {
		return "commodity"
	}

	return "other"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
