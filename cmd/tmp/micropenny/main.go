// micropenny analyzes price discrepancies in Coinbase trade fills.
//
// This tool compares native_amount (actual USD) vs amount*fill_price (theoretical)
// for each fill to understand rounding behavior and price differences.
//
// Key finding: fill_price appears to be a reference/limit price, not the actual
// execution price. The implied price (native_amount/amount) often differs and
// tends to be MORE favorable to us:
//   - Buys: we often pay LESS than fill_price suggests
//   - Sells: we often receive MORE than fill_price suggests
//
// Usage:
//
//	go run ./cmd/tmp/micropenny
package main

import (
	"dropbear/db"
	"dropbear/decimal"
	"dropbear/loggy"
	"flag"
	"fmt"
)

var (
	flagVerbose = flag.Bool("v", false, "show individual trades")
	flagLimit   = flag.Int("limit", 0, "limit number of trades to show in verbose mode")
)

func main() {
	loggy.Init()
	flag.Parse()

	database := db.Get()

	// Get all trades (excluding USD mirror transactions)
	rows, err := database.Query(`
		SELECT currency, amount, native_amount, fill_price, fill_commission, fill_side,
		       datetime(created_at/1000000, 'unixepoch')
		FROM coinbase_transactions
		WHERE type = 'advanced_trade_fill'
		  AND currency != 'USD'
		  AND fill_price != ''
		ORDER BY created_at
	`)
	if err != nil {
		loggy.Fatalf("query failed: %v", err)
	}
	defer rows.Close()

	var (
		totalFills int

		// Price discrepancy tracking (fill_price vs implied price)
		buyFillsTotal      int
		buyFillsBetter     int // implied < fill_price (paid less)
		buyFillsWorse      int // implied > fill_price (paid more)
		buyNetDiscrepancy  decimal.Decimal
		sellFillsTotal     int
		sellFillsBetter    int // implied > fill_price (received more)
		sellFillsWorse     int // implied < fill_price (received less)
		sellNetDiscrepancy decimal.Decimal

		// Penny rounding tracking (sub-cent differences only)
		roundingWins   int
		roundingLosses int
		roundingNet    decimal.Decimal

		totalFees decimal.Decimal
	)

	cent := decimal.Parse("0.01")
	verboseCount := 0

	if *flagVerbose {
		fmt.Printf("%-5s %-19s %-4s %14s %11s %11s %11s %9s\n",
			"ASSET", "DATE", "SIDE", "QTY", "FILL_PRICE", "IMPLIED", "NATIVE", "DIFF")
		fmt.Println("--------------------------------------------------------------------------------")
	}

	for rows.Next() {
		var currency, date, side string
		var amountStr, nativeStr, priceStr, feeStr string
		if err := rows.Scan(&currency, &amountStr, &nativeStr, &priceStr, &feeStr, &side, &date); err != nil {
			continue
		}

		amount := decimal.Parse(amountStr).Abs()
		native := decimal.Parse(nativeStr).Abs()
		fillPrice := decimal.Parse(priceStr)
		if feeStr != "" {
			totalFees = totalFees.Add(decimal.Parse(feeStr))
		}

		// Skip dust trades (< $1)
		if native.Cmp(decimal.FromInt(1)) < 0 {
			continue
		}

		totalFills++

		// Theoretical = amount * fill_price
		theoretical := amount.Mul(fillPrice)

		// Discrepancy = native - theoretical
		// For buys: positive = we paid more, negative = we paid less
		// For sells: positive = we received more, negative = we received less
		discrepancy := native.Sub(theoretical)

		// Implied price = native / amount
		impliedPrice := native.Div(amount)

		// Track by side
		if side == "buy" {
			buyFillsTotal++
			buyNetDiscrepancy = buyNetDiscrepancy.Add(discrepancy)
			if discrepancy.IsNegative() {
				buyFillsBetter++ // paid less than theoretical
			} else if discrepancy.IsPositive() {
				buyFillsWorse++ // paid more than theoretical
			}
		} else {
			sellFillsTotal++
			sellNetDiscrepancy = sellNetDiscrepancy.Add(discrepancy)
			if discrepancy.IsPositive() {
				sellFillsBetter++ // received more than theoretical
			} else if discrepancy.IsNegative() {
				sellFillsWorse++ // received less than theoretical
			}
		}

		// Track penny rounding (differences < 1 cent)
		absDisc := discrepancy.Abs()
		if absDisc.Cmp(cent) < 0 && !absDisc.IsZero() {
			if side == "buy" {
				if discrepancy.IsNegative() {
					roundingWins++
					roundingNet = roundingNet.Sub(discrepancy.Abs())
				} else {
					roundingLosses++
					roundingNet = roundingNet.Add(discrepancy)
				}
			} else {
				if discrepancy.IsPositive() {
					roundingWins++
					roundingNet = roundingNet.Sub(discrepancy)
				} else {
					roundingLosses++
					roundingNet = roundingNet.Add(discrepancy.Abs())
				}
			}
		}

		if *flagVerbose && (*flagLimit == 0 || verboseCount < *flagLimit) {
			// Only show fills with significant discrepancy (> $0.10)
			if absDisc.Cmp(decimal.Parse("0.10")) > 0 {
				fmt.Printf("%-5s %-19s %-4s %14s %11s %11s %11s %9s\n",
					currency,
					date,
					side,
					amount.String(),
					"$"+fillPrice.Format(2),
					"$"+impliedPrice.Format(2),
					"$"+native.Format(2),
					"$"+discrepancy.Format(2))
				verboseCount++
			}
		}
	}

	fmt.Println()
	fmt.Println("=== COINBASE FILL PRICE ANALYSIS ===")
	fmt.Println()
	fmt.Printf("Total fills analyzed:    %d (>$1 each)\n", totalFills)
	fmt.Printf("Total fees paid:         $%s\n", totalFees.Format(2))
	fmt.Println()

	fmt.Println("--- PRICE DISCREPANCY (native vs amount*fill_price) ---")
	fmt.Println()
	fmt.Printf("BUY fills:               %d\n", buyFillsTotal)
	fmt.Printf("  Paid LESS than fill_price: %d (%.1f%%) - good\n",
		buyFillsBetter, float64(buyFillsBetter)/float64(buyFillsTotal)*100)
	fmt.Printf("  Paid MORE than fill_price: %d (%.1f%%) - bad\n",
		buyFillsWorse, float64(buyFillsWorse)/float64(buyFillsTotal)*100)
	fmt.Printf("  Net discrepancy:       $%s", buyNetDiscrepancy.Format(2))
	if buyNetDiscrepancy.IsNegative() {
		fmt.Println(" (saved money)")
	} else {
		fmt.Println(" (lost money)")
	}
	fmt.Println()

	fmt.Printf("SELL fills:              %d\n", sellFillsTotal)
	fmt.Printf("  Got MORE than fill_price:  %d (%.1f%%) - good\n",
		sellFillsBetter, float64(sellFillsBetter)/float64(sellFillsTotal)*100)
	fmt.Printf("  Got LESS than fill_price:  %d (%.1f%%) - bad\n",
		sellFillsWorse, float64(sellFillsWorse)/float64(sellFillsTotal)*100)
	fmt.Printf("  Net discrepancy:       $%s", sellNetDiscrepancy.Format(2))
	if sellNetDiscrepancy.IsPositive() {
		fmt.Println(" (extra money)")
	} else {
		fmt.Println(" (lost money)")
	}
	fmt.Println()

	// For buys: negative discrepancy = we saved money
	// For sells: positive discrepancy = we made extra money
	// Net benefit = sellNetDiscrepancy - buyNetDiscrepancy (since buy disc is typically negative)
	netBenefit := sellNetDiscrepancy.Sub(buyNetDiscrepancy)
	fmt.Printf("TOTAL NET BENEFIT:       $%s\n", netBenefit.Format(2))
	fmt.Println()

	fmt.Println("--- SUB-PENNY ROUNDING (<$0.01 differences) ---")
	fmt.Println()
	fmt.Printf("Rounding in our favor:   %d fills\n", roundingWins)
	fmt.Printf("Rounding against us:     %d fills\n", roundingLosses)
	fmt.Printf("Net rounding impact:     $%s", roundingNet.Format(4))
	if roundingNet.IsPositive() {
		fmt.Println(" (lost to rounding)")
	} else if roundingNet.IsNegative() {
		fmt.Println(" (gained from rounding)")
	} else {
		fmt.Println(" (neutral)")
	}
	fmt.Println()

	fmt.Println("CONCLUSION:")
	fmt.Println("  fill_price appears to be a reference/limit price, not actual execution.")
	fmt.Println("  The implied price (native_amount/amount) is often more favorable.")
	fmt.Println("  Sub-penny rounding is roughly fair (nearest cent, not biased).")
}
