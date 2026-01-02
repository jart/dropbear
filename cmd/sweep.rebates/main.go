// sweep.rebates transfers fee rebates from the primary portfolio to trading portfolios.
//
// Coinbase VIP members receive a 25% rebate on trading fees, credited as USDC to the
// primary portfolio. This command calculates the rebates owed to each trading portfolio
// based on their commission history, transfers the USDC, and converts it to USD.
//
// The rebate formula is: floor(commission * 0.25, 6 decimal places)
//
// Usage:
//
//	go run ./cmd/sweep.rebates
//	go run ./cmd/sweep.rebates -dry  # dry run, don't actually transfer
package main

import (
	"database/sql"
	"dropbear/broker/coinbase"
	"dropbear/db"
	"dropbear/decimal"
	"dropbear/loggy"
	"flag"
	"log"
	"os"
)

var (
	dryRun = flag.Bool("dry", false, "dry run, don't actually transfer")
)

// TradingPortfolio represents a trading portfolio that should receive rebates.
type TradingPortfolio struct {
	Name    string // human-readable name
	KeyPath string // path to API key
	DBPath  string // path to database
}

// tradingPortfolios is the list of trading portfolios.
var tradingPortfolios = []TradingPortfolio{
	{"BTC", "$HOME/.coinbase.key", "$HOME/.dropbear.sqlite3"},
	{"ZEC", "$HOME/.zec.key", "$HOME/.zec.sqlite3"},
}

// rebateQuantum is the precision for rebate calculations (6 decimal places).
var rebateQuantum = decimal.Parse("0.000001")

// rebateRate is the VIP rebate percentage (25%).
var rebateRate = decimal.Parse("0.25")

// minTransfer is the minimum amount to bother transferring.
var minTransfer = decimal.Parse("1.00")

func main() {
	flag.Parse()
	loggy.Init()

	// Load primary portfolio client
	primaryDB, err := db.Open(os.ExpandEnv("$HOME/.primary.sqlite3"))
	if err != nil {
		loggy.Fatalf("could not open primary database: %v", err)
	}
	defer primaryDB.Close()
	primaryKey, err := coinbase.LoadKey(os.ExpandEnv("$HOME/.primary.key"))
	if err != nil {
		loggy.Fatalf("could not load primary key: %v", err)
	}
	primaryClient := coinbase.NewClientWith(primaryKey, primaryDB)
	defer primaryClient.Close()

	// Get primary portfolio info
	primaryAccounts, err := primaryClient.GetAccounts()
	if err != nil {
		loggy.Fatalf("could not get primary accounts: %v", err)
	}
	var primaryPortfolioUUID string
	var primaryUSDAccountUUID string
	var primaryUSDCAccountUUID string
	var primaryUSDCBalance decimal.Decimal
	for _, acc := range primaryAccounts {
		if primaryPortfolioUUID == "" {
			primaryPortfolioUUID = acc.RetailPortfolioID
		}
		if acc.Currency == "USD" {
			primaryUSDAccountUUID = acc.UUID
		}
		if acc.Currency == "USDC" {
			primaryUSDCAccountUUID = acc.UUID
			primaryUSDCBalance = decimal.Parse(acc.AvailableBalance.Value)
			log.Printf("primary USDC balance: %s (account %s)", primaryUSDCBalance, primaryUSDCAccountUUID)
		}
	}
	if primaryPortfolioUUID == "" {
		loggy.Fatalf("could not find primary portfolio UUID")
	}
	if primaryUSDAccountUUID == "" {
		loggy.Fatalf("could not find primary USD account")
	}
	log.Printf("primary portfolio UUID: %s", primaryPortfolioUUID)

	// Process each trading portfolio
	for _, tp := range tradingPortfolios {
		processPortfolio(primaryClient, primaryPortfolioUUID, &primaryUSDCBalance, tp)
	}

	log.Printf("done, remaining primary USDC balance: %s", primaryUSDCBalance)
}

func processPortfolio(primaryClient *coinbase.Client, primaryPortfolioUUID string, primaryUSDCBalance *decimal.Decimal, tp TradingPortfolio) {
	log.Printf("processing %s portfolio", tp.Name)

	// Open trading portfolio database
	tradingDB, err := db.Open(os.ExpandEnv(tp.DBPath))
	if err != nil {
		log.Printf("warning: could not open %s database: %v", tp.Name, err)
		return
	}
	defer tradingDB.Close()

	// Load trading portfolio client
	tradingKey, err := coinbase.LoadKey(os.ExpandEnv(tp.KeyPath))
	if err != nil {
		log.Printf("warning: could not load %s key: %v", tp.Name, err)
		return
	}
	tradingClient := coinbase.NewClientWith(tradingKey, tradingDB)
	defer tradingClient.Close()

	// Get trading portfolio info
	tradingAccounts, err := tradingClient.GetAccounts()
	if err != nil {
		log.Printf("warning: could not get %s accounts: %v", tp.Name, err)
		return
	}
	var tradingPortfolioUUID string
	for _, acc := range tradingAccounts {
		if tradingPortfolioUUID == "" {
			tradingPortfolioUUID = acc.RetailPortfolioID
			break
		}
	}
	if tradingPortfolioUUID == "" {
		log.Printf("warning: could not find %s portfolio UUID", tp.Name)
		return
	}
	log.Printf("%s portfolio UUID: %s", tp.Name, tradingPortfolioUUID)

	// Calculate expected rebates from commission history
	expectedRebates := calculateExpectedRebates(tradingDB)
	log.Printf("%s expected rebates: %s", tp.Name, expectedRebates)

	// Calculate USDC already received (from prior sweeps)
	// This is the sum of positive USDC amounts in the trading portfolio
	// (since USDC in trading portfolios is ONLY from rebate transfers)
	receivedRebates := calculateReceivedRebates(tradingDB)
	log.Printf("%s received rebates: %s", tp.Name, receivedRebates)

	// Calculate deficit
	deficit := expectedRebates.Sub(receivedRebates)
	log.Printf("%s rebate deficit: %s", tp.Name, deficit)

	if deficit.Cmp(minTransfer) < 0 {
		log.Printf("%s deficit below minimum transfer threshold, skipping", tp.Name)
		return
	}

	// Check if primary has enough USDC
	if primaryUSDCBalance.Cmp(deficit) < 0 {
		log.Printf("warning: primary USDC balance (%s) insufficient for %s deficit (%s)",
			*primaryUSDCBalance, tp.Name, deficit)
		deficit = *primaryUSDCBalance
		if deficit.Cmp(minTransfer) < 0 {
			log.Printf("adjusted deficit below minimum, skipping")
			return
		}
	}

	// Quantize to 6 decimal places (USDC precision)
	deficit = deficit.QuantizeTruncate(rebateQuantum)

	if *dryRun {
		log.Printf("[DRY RUN] would convert %s USDC to USD in primary", deficit)
		log.Printf("[DRY RUN] would transfer %s USD from primary to %s", deficit, tp.Name)
		return
	}

	// Convert USDC to USD in primary portfolio first
	// (Coinbase only allows conversion on default/primary portfolio)
	log.Printf("converting %s USDC to USD in primary", deficit)
	trade, err := primaryClient.ConvertUSDCtoUSD(deficit)
	if err != nil {
		log.Printf("error: failed to convert USDC to USD: %v", err)
		return
	}
	*primaryUSDCBalance = primaryUSDCBalance.Sub(deficit)
	log.Printf("converted %s USDC to USD (trade ID: %s)", deficit, trade.ID)

	// Transfer USD from primary to trading portfolio
	log.Printf("transferring %s USD from primary to %s", deficit, tp.Name)
	err = primaryClient.MovePortfolioFunds("USD", deficit, primaryPortfolioUUID, tradingPortfolioUUID)
	if err != nil {
		log.Printf("error: failed to transfer USD: %v", err)
		return
	}
	log.Printf("transferred %s USD to %s", deficit, tp.Name)

	// Sync USD transactions to record the transfer
	if err := tradingClient.SyncTransactions("USD"); err != nil {
		log.Printf("warning: could not sync USD transactions: %v", err)
	}
}

// calculateExpectedRebates computes the total rebates owed based on commission history.
// Uses the exact Coinbase formula: floor(commission * 0.25, 6 decimals) per transaction.
func calculateExpectedRebates(database *sql.DB) decimal.Decimal {
	rows, err := database.Query(`
		SELECT fill_commission
		FROM coinbase_transactions
		WHERE fill_commission IS NOT NULL
		  AND fill_commission != ''
		  AND currency != 'USD'
		  AND status = 'completed'
	`)
	if err != nil {
		log.Printf("warning: could not query commissions: %v", err)
		return decimal.Zero
	}
	defer rows.Close()

	total := decimal.Zero
	for rows.Next() {
		var commissionStr string
		if err := rows.Scan(&commissionStr); err != nil {
			continue
		}
		commission := decimal.Parse(commissionStr)
		// Coinbase rebate formula: floor(commission * 0.25, 6 decimals)
		rebate := commission.Mul(rebateRate).QuantizeTruncate(rebateQuantum)
		total = total.Add(rebate)
	}
	return total
}

// maxRebateSweep is the maximum size of a single rebate sweep.
// This distinguishes rebate sweeps from capital deposits.
var maxRebateSweep = decimal.Parse("500")

// calculateReceivedRebates computes how much rebate USD has been transferred.
// We track USD transfers (type='tx') that are small amounts (rebate sweeps).
// Large transfers are capital deposits and should be excluded.
func calculateReceivedRebates(database *sql.DB) decimal.Decimal {
	// Sum small USD transfers (rebate sweeps from primary)
	// These are type='tx' with positive amounts under $500
	var transferred decimal.Decimal
	row := database.QueryRow(`
		SELECT COALESCE(SUM(CAST(amount AS REAL)), 0)
		FROM coinbase_transactions
		WHERE currency = 'USD'
		  AND status = 'completed'
		  AND type = 'tx'
		  AND CAST(amount AS REAL) > 0
		  AND CAST(amount AS REAL) < 500
	`)
	var sum float64
	if err := row.Scan(&sum); err != nil {
		log.Printf("warning: could not query USD transfers: %v", err)
	} else {
		transferred = decimal.FromFloat64(sum)
	}

	log.Printf("  USD rebate sweeps received: %s", transferred)
	return transferred
}
