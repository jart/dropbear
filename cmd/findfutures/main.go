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
	flagSync      = flag.Bool("sync", false, "fetch latest data from Databento")
	flagAsset     = flag.String("asset", "", "filter to specific asset (e.g., 6J, ES, NQ)")
	flagType      = flag.String("type", "", "filter by type: currency, index, energy, metal, ag, rate, vol, crypto")
	flagMinOI     = flag.Int("min-oi", 0, "minimum open interest (default 0, set higher to filter)")
	flagUSRate    = flag.Float64("us-rate", 4.5, "current US risk-free rate (for implied foreign rate calc)")
	flagSimple    = flag.Bool("simple", false, "show simple output: one contract per asset to trade")
	flagFinancial = flag.Bool("financial", false, "only show financial futures (FX, rates, index) where carry is tradeable")
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
// Note: margins vary by expiration month; these are approximate front-month values
var marginRequirements = map[string]float64{
	// ===== FX - G10 =====
	"6J": 2800, "6E": 2700, "6S": 4500, "6A": 1900, "6B": 2000,
	"6C": 1000, "6N": 1300, "6M": 2500,
	"E7": 1350, "J7": 1400, // E-mini FX
	"M6E": 270, "M6A": 190, "M6B": 200, "MCD": 100, "MSF": 450, "MJY": 280, // Micro FX

	// ===== FX - EM =====
	"6L": 4000, "6R": 5000, "6Z": 3000, "PLS": 2000, "CZK": 1500,
	"HUF": 2000, "ILS": 1500, "KRW": 2000, "CNH": 3500,

	// ===== EQUITY INDEX =====
	"ES": 22731, "NQ": 33563, "YM": 14245, "RTY": 9491, "EMD": 15000, "NKD": 12000, "NIY": 1200000, // Note: NIY is JPY-denominated
	"MES": 2273, "MNQ": 3356, "MYM": 1424, "M2K": 949, // Micro index (1/10th of E-mini)
	"XAF": 2000, "XAK": 3500, "XAE": 4000, "XAU": 1500, "XAV": 2500,
	"XAB": 1500, "XAY": 2500, "XAI": 2000, "XAP": 2000, // Sector futures

	// ===== INTEREST RATES =====
	"ZT": 1000, "Z3N": 1200, "ZF": 1500, "ZN": 2000, "TN": 2500, "ZB": 3500, "UB": 5000,
	"SR3": 500, "SR1": 300, "FF": 300, "GE": 500,

	// ===== ENERGY =====
	"CL": 6500, "QM": 3250, "MCL": 650, // WTI Crude
	"BZ": 7000,                          // Brent
	"NG": 3000, "QG": 1500,              // Natural Gas
	"HO": 7500, "RB": 7000,              // Refined products

	// ===== METALS =====
	"GC": 24881, "SI": 45417, "HG": 10000, "PL": 3500, "PA": 15000,
	"MGC": 2488, "SIL": 9000, "QC": 2500, "ALI": 3000,

	// ===== AGRICULTURE =====
	"ZC": 975, "ZS": 2000, "ZW": 1650, "KE": 1800, "ZO": 800,
	"ZM": 1800, "ZL": 1200,
	"XC": 195, "XW": 330, "XK": 400, // Mini grains

	// ===== LIVESTOCK =====
	"LE": 2200, "GF": 3500, "HE": 1800,

	// ===== DAIRY =====
	"DC": 1500, "DY": 800, "CB": 2000, "CSC": 1500,

	// ===== OTHER =====
	"LBS": 3500, // Lumber
	"VX": 8000,  // VIX

	// ===== CRYPTO =====
	"BTC": 50000, "MBT": 5000, "ETH": 8000, "MET": 800,
}

// Contract multipliers for calculating notional value
// For products quoted in cents, the multiplier is adjusted (÷100)
var contractMultipliers = map[string]float64{
	// ===== FX - G10 (units of foreign currency) =====
	"6J": 12500000, "6E": 125000, "6S": 125000, "6A": 100000, "6B": 62500,
	"6C": 100000, "6N": 100000, "6M": 500000,
	"E7": 62500, "J7": 6250000, // E-mini FX (half size)
	"M6E": 12500, "M6A": 10000, "M6B": 6250, "MCD": 10000, "MSF": 12500, "MJY": 1250000, // Micro FX (1/10th)

	// ===== FX - EM =====
	"6L": 100000, "6R": 2500000, "6Z": 500000, "PLS": 500000, "CZK": 4000000,
	"HUF": 30000000, "ILS": 1000000, "KRW": 125000000, "CNH": 100000,

	// ===== EQUITY INDEX (multiplier × index value) =====
	"ES": 50, "NQ": 20, "YM": 5, "RTY": 50, "EMD": 100, "NKD": 5, "NIY": 500,
	"MES": 5, "MNQ": 2, "MYM": 0.5, "M2K": 5, // Micro (1/10th)
	"XAF": 100, "XAK": 100, "XAE": 100, "XAU": 100, "XAV": 100,
	"XAB": 100, "XAY": 100, "XAI": 100, "XAP": 100,

	// ===== INTEREST RATES (face value / 1000 for price display) =====
	"ZT": 2000, "Z3N": 1000, "ZF": 1000, "ZN": 1000, "TN": 1000, "ZB": 1000, "UB": 1000,
	"SR3": 2500, "SR1": 4167, "FF": 4167, "GE": 2500, // $1M notional contracts

	// ===== ENERGY =====
	"CL": 1000, "QM": 500, "MCL": 100,  // Crude (barrels)
	"BZ": 1000,                          // Brent
	"NG": 10000, "QG": 2500,             // Natural Gas (MMBtu)
	"HO": 42000, "RB": 42000,            // 42,000 gallons

	// ===== METALS =====
	"GC": 100, "SI": 5000, "HG": 25000, "PL": 50, "PA": 100,
	"MGC": 10, "SIL": 1000, "QC": 12500, "ALI": 25,

	// ===== AGRICULTURE (bushels, quoted in cents so ÷100) =====
	"ZC": 50, "ZS": 50, "ZW": 50, "KE": 50, "ZO": 50, // 5000 bu ÷ 100
	"ZM": 1,  // 100 short tons, price in $/ton
	"ZL": 0.6, // 60,000 lbs, price in cents/lb
	"XC": 10, "XW": 10, "XK": 10, // Mini grains (1000 bu ÷ 100)

	// ===== LIVESTOCK (cents per lb, so ÷100) =====
	"LE": 400, "GF": 500, "HE": 400, // 40,000 lbs ÷ 100

	// ===== DAIRY =====
	"DC": 2000, "DY": 440, "CB": 200, "CSC": 200, // Cwt or lbs

	// ===== OTHER =====
	"LBS": 27.5, // 27,500 board feet, price in $/MBF
	"VX": 1000,  // $1000 × VIX

	// ===== CRYPTO =====
	"BTC": 5, "MBT": 0.1, "ETH": 50, "MET": 0.1, // Coins per contract
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
		// Financial filter: only show futures where carry is actually tradeable
		// Physical commodities have "carry" that reflects storage costs, not arbitrage opportunity
		if *flagFinancial {
			switch opp.AssetType {
			case "currency", "rate", "index":
				// These are cash-settled, carry is tradeable
			default:
				continue // Skip physical commodities, crypto, vol
			}
		}
		filtered = append(filtered, opp)
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
			"CONTRACT", "TYPE", "PRICE", "MARGIN", "YIELD%", "RET/MARGIN", "ACTION")
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
			// Show absolute carry since ACTION tells you the direction
			absCarry := opp.AnnualizedCarry
			if absCarry < 0 {
				absCarry = -absCarry
			}
			fmt.Printf("%-10s %-8s %12.6f %10s %7.2f%% %10s %-5s\n",
				opp.FrontSymbol,
				opp.AssetType[:min(8, len(opp.AssetType))],
				opp.FrontPrice,
				marginStr,
				absCarry*100,
				retOnMargin,
				action)
		}

		if len(simple) == 0 {
			fmt.Println("No opportunities found matching criteria.")
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
	// Generate symbols for CME Globex futures
	// Format: ASSET + MONTH_CODE + YEAR_DIGIT
	// Month codes: F=Jan, G=Feb, H=Mar, J=Apr, K=May, M=Jun, N=Jul, Q=Aug, U=Sep, V=Oct, X=Nov, Z=Dec

	assets := []struct {
		name   string
		months string // which months have contracts
	}{
		// ===== EQUITY INDEX =====
		// E-mini index futures (quarterly)
		{"ES", "HMUZ"},  // E-mini S&P 500
		{"NQ", "HMUZ"},  // E-mini Nasdaq 100
		{"YM", "HMUZ"},  // E-mini Dow Jones
		{"RTY", "HMUZ"}, // E-mini Russell 2000
		{"EMD", "HMUZ"}, // E-mini S&P MidCap 400
		{"NKD", "HMUZ"}, // Nikkei 225 (USD)
		{"NIY", "HMUZ"}, // Nikkei 225 (Yen)

		// Micro index futures (quarterly)
		{"MES", "HMUZ"}, // Micro E-mini S&P 500
		{"MNQ", "HMUZ"}, // Micro E-mini Nasdaq 100
		{"MYM", "HMUZ"}, // Micro E-mini Dow
		{"M2K", "HMUZ"}, // Micro E-mini Russell 2000

		// Sector futures
		{"XAF", "HMUZ"}, // E-mini Financial Select
		{"XAK", "HMUZ"}, // E-mini Technology Select
		{"XAE", "HMUZ"}, // E-mini Energy Select
		{"XAU", "HMUZ"}, // E-mini Utilities Select
		{"XAV", "HMUZ"}, // E-mini Health Care Select
		{"XAB", "HMUZ"}, // E-mini Consumer Staples
		{"XAY", "HMUZ"}, // E-mini Consumer Discretionary
		{"XAI", "HMUZ"}, // E-mini Industrial Select
		{"XAP", "HMUZ"}, // E-mini Materials Select

		// ===== FX - G10 =====
		// Standard FX (quarterly, some monthly)
		{"6E", "FGHJKMNQUVXZ"}, // Euro
		{"6J", "FGHJKMNQUVXZ"}, // Japanese Yen
		{"6B", "FGHJKMNQUVXZ"}, // British Pound
		{"6C", "FGHJKMNQUVXZ"}, // Canadian Dollar
		{"6A", "FGHJKMNQUVXZ"}, // Australian Dollar
		{"6S", "FGHJKMNQUVXZ"}, // Swiss Franc
		{"6N", "HMUZ"},         // New Zealand Dollar
		{"6M", "FGHJKMNQUVXZ"}, // Mexican Peso

		// E-mini FX
		{"E7", "HMUZ"}, // E-mini Euro
		{"J7", "HMUZ"}, // E-mini Japanese Yen

		// Micro FX
		{"M6E", "HMUZ"}, // Micro Euro
		{"M6A", "HMUZ"}, // Micro AUD
		{"M6B", "HMUZ"}, // Micro GBP
		{"MCD", "HMUZ"}, // Micro CAD
		{"MSF", "HMUZ"}, // Micro CHF
		{"MJY", "HMUZ"}, // Micro JPY

		// ===== FX - EM =====
		{"6L", "FGHJKMNQUVXZ"}, // Brazilian Real
		{"6R", "HMUZ"},         // Russian Ruble (may be delisted)
		{"6Z", "HMUZ"},         // South African Rand
		{"PLS", "HMUZ"},        // Polish Zloty
		{"CZK", "HMUZ"},        // Czech Koruna
		{"HUF", "HMUZ"},        // Hungarian Forint
		{"ILS", "HMUZ"},        // Israeli Shekel
		{"KRW", "HMUZ"},        // Korean Won
		{"CNH", "FGHJKMNQUVXZ"}, // Offshore Chinese Yuan

		// ===== INTEREST RATES =====
		// Treasury futures (quarterly)
		{"ZT", "HMUZ"}, // 2-Year T-Note
		{"Z3N", "HMUZ"}, // 3-Year T-Note
		{"ZF", "HMUZ"}, // 5-Year T-Note
		{"ZN", "HMUZ"}, // 10-Year T-Note
		{"TN", "HMUZ"}, // Ultra 10-Year T-Note
		{"ZB", "HMUZ"}, // 30-Year T-Bond
		{"UB", "HMUZ"}, // Ultra T-Bond

		// SOFR futures (monthly and quarterly)
		{"SR3", "FGHJKMNQUVXZ"}, // 3-Month SOFR
		{"SR1", "FGHJKMNQUVXZ"}, // 1-Month SOFR

		// Fed Funds
		{"FF", "FGHJKMNQUVXZ"}, // 30-Day Fed Funds

		// Eurodollars (being phased out but still trade)
		{"GE", "HMUZ"}, // Eurodollar

		// ===== ENERGY - NYMEX =====
		// Crude Oil
		{"CL", "FGHJKMNQUVXZ"}, // WTI Crude Oil
		{"QM", "FGHJKMNQUVXZ"}, // E-mini Crude Oil
		{"MCL", "FGHJKMNQUVXZ"}, // Micro WTI Crude

		// Brent
		{"BZ", "FGHJKMNQUVXZ"}, // Brent Crude (last day)

		// Natural Gas
		{"NG", "FGHJKMNQUVXZ"}, // Henry Hub Natural Gas
		{"QG", "FGHJKMNQUVXZ"}, // E-mini Natural Gas

		// Refined Products
		{"HO", "FGHJKMNQUVXZ"}, // NY Harbor ULSD (Heating Oil)
		{"RB", "FGHJKMNQUVXZ"}, // RBOB Gasoline

		// ===== METALS - COMEX =====
		// Precious Metals
		{"GC", "FGHJKMNQUVXZ"}, // Gold
		{"SI", "FHKNUZ"},       // Silver
		{"PL", "FJNV"},         // Platinum
		{"PA", "HMUZ"},         // Palladium

		// Micro/Mini Metals
		{"MGC", "FGHJKMNQUVXZ"}, // Micro Gold
		{"SIL", "FGHJKMNQUVXZ"}, // Micro Silver (1000 oz)

		// Base Metals
		{"HG", "FHKNUZ"},       // Copper
		{"ALI", "FGHJKMNQUVXZ"}, // Aluminum
		{"QC", "FHKNUZ"},       // E-mini Copper

		// ===== AGRICULTURE - CBOT =====
		// Grains
		{"ZC", "FHKNUZ"},       // Corn
		{"ZS", "FHKNQUX"},      // Soybeans
		{"ZW", "FHKNUZ"},       // Wheat (Chicago SRW)
		{"KE", "FHKNUZ"},       // KC HRW Wheat
		{"ZO", "FHKNUZ"},       // Oats

		// Soybean Complex
		{"ZM", "FHKNQUVZ"},     // Soybean Meal
		{"ZL", "FHKNQUVZ"},     // Soybean Oil

		// Mini Grains
		{"XC", "FHKNUZ"},       // Mini Corn
		{"XW", "FHKNUZ"},       // Mini Wheat
		{"XK", "FHKNQUX"},      // Mini Soybeans

		// ===== LIVESTOCK - CME =====
		{"LE", "GJMQVZ"},       // Live Cattle
		{"GF", "FHJKQUVX"},     // Feeder Cattle
		{"HE", "GJKMNQVZ"},     // Lean Hogs

		// ===== DAIRY - CME =====
		{"DC", "FGHJKMNQUVXZ"}, // Class III Milk
		{"DY", "FGHJKMNQUVXZ"}, // Dry Whey
		{"CB", "FGHJKMNQUVXZ"}, // Cash-Settled Butter
		{"CSC", "FGHJKMNQUVXZ"}, // Cash-Settled Cheese

		// ===== LUMBER/SOFTS =====
		{"LBS", "FHKNUX"},      // Random Length Lumber

		// ===== WEATHER =====
		{"H1", "FGHJKVXZ"},     // HDD (Heating Degree Day)
		{"K1", "JKMNQV"},       // CDD (Cooling Degree Day)

		// ===== VOLATILITY =====
		{"VX", "FGHJKMNQUVXZ"}, // VIX Futures

		// ===== BITCOIN/CRYPTO =====
		{"BTC", "FGHJKMNQUVXZ"}, // Bitcoin Futures
		{"MBT", "FGHJKMNQUVXZ"}, // Micro Bitcoin
		{"ETH", "FGHJKMNQUVXZ"}, // Ether Futures
		{"MET", "FGHJKMNQUVXZ"}, // Micro Ether
	}

	// Generate for current year, next year, and year after
	currentYear := time.Now().Year()
	years := []int{currentYear % 10, (currentYear + 1) % 10, (currentYear + 2) % 10}

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
				// Return on margin = |carry| × (notional / margin)
				// This is the leveraged return from taking the correct direction
				absCarry := carry
				if absCarry < 0 {
					absCarry = -absCarry
				}
				returnOnMargin = absCarry * (notional / margin)
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
	// Currency futures typically start with 6, E7/J7, or M6/M prefix for micro
	if strings.HasPrefix(asset, "6") || asset == "E7" || asset == "J7" ||
		strings.HasPrefix(asset, "M6") || asset == "MCD" || asset == "MSF" || asset == "MJY" ||
		asset == "CNH" || asset == "6L" || asset == "6R" || asset == "6Z" ||
		asset == "PLS" || asset == "CZK" || asset == "HUF" || asset == "ILS" || asset == "KRW" {
		return "currency"
	}

	// Equity index futures
	indexAssets := map[string]bool{
		"ES": true, "NQ": true, "YM": true, "RTY": true, "EMD": true, "NKD": true, "NIY": true,
		"MES": true, "MNQ": true, "MYM": true, "M2K": true,
		"XAF": true, "XAK": true, "XAE": true, "XAU": true, "XAV": true,
		"XAB": true, "XAY": true, "XAI": true, "XAP": true,
	}
	if indexAssets[asset] {
		return "index"
	}

	// Interest rate futures
	rateAssets := map[string]bool{
		"ZT": true, "Z3N": true, "ZF": true, "ZN": true, "TN": true, "ZB": true, "UB": true,
		"GE": true, "SR3": true, "SR1": true, "FF": true,
	}
	if rateAssets[asset] {
		return "rate"
	}

	// Energy futures
	energyAssets := map[string]bool{
		"CL": true, "QM": true, "MCL": true, "BZ": true,
		"NG": true, "QG": true, "HO": true, "RB": true,
	}
	if energyAssets[asset] {
		return "energy"
	}

	// Metals
	metalAssets := map[string]bool{
		"GC": true, "SI": true, "HG": true, "PL": true, "PA": true,
		"MGC": true, "SIL": true, "QC": true, "ALI": true,
	}
	if metalAssets[asset] {
		return "metal"
	}

	// Agricultural
	agAssets := map[string]bool{
		"ZC": true, "ZS": true, "ZW": true, "ZM": true, "ZL": true, "KE": true, "ZO": true,
		"XC": true, "XW": true, "XK": true,
		"LE": true, "HE": true, "GF": true,
		"DC": true, "DY": true, "CB": true, "CSC": true,
		"LBS": true,
	}
	if agAssets[asset] {
		return "ag"
	}

	// Volatility
	if asset == "VX" {
		return "vol"
	}

	// Crypto
	if asset == "BTC" || asset == "MBT" || asset == "ETH" || asset == "MET" {
		return "crypto"
	}

	return "other"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
