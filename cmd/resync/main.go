package main

import (
	"database/sql"
	"dropbear/db"
	"dropbear/exchange/coinbase"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

func main() {
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: resync <currency>\n")
		fmt.Fprintf(os.Stderr, "example: resync BTC\n")
		os.Exit(1)
	}
	currency := flag.Arg(0)

	database := db.Get()
	client := coinbase.NewClient()

	// Get v2 account for this currency
	account, err := client.GetV2AccountByCurrencyCode(currency)
	if err != nil {
		log.Fatalf("getting account for %s: %v", currency, err)
	}

	// Ensure table exists
	if _, err := database.Exec(coinbase.TransactionsSchema()); err != nil {
		log.Fatalf("creating table: %v", err)
	}

	// Start SQL transaction for atomic resync
	tx, err := database.Begin()
	if err != nil {
		log.Fatalf("beginning transaction: %v", err)
	}
	defer tx.Rollback()

	// Delete all existing transactions for this account
	result, err := tx.Exec(`
		DELETE FROM coinbase_transactions WHERE account_id = :account_id
	`, sql.Named("account_id", account.ID))
	if err != nil {
		log.Fatalf("deleting transactions: %v", err)
	}
	deleted, _ := result.RowsAffected()
	log.Printf("deleted %d existing transactions for account %s", deleted, account.ID)

	// Prepare insert statement
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO coinbase_transactions (
			id, account_id, type, status, created_at, updated_at,
			amount, currency, native_amount, native_currency,
			fill_order_id, fill_product_id, fill_side, fill_price, fill_commission, idem,
			network_hash, network_name, network_fee,
			to_resource, to_address, to_id, to_email, to_name,
			from_resource, from_address, from_id, from_email, from_name,
			buy_id, buy_fee, buy_total, buy_subtotal, buy_payment_method_name,
			sell_id, sell_fee, sell_total, sell_subtotal, sell_payment_method_name
		) VALUES (
			:id, :account_id, :type, :status, :created_at, :updated_at,
			:amount, :currency, :native_amount, :native_currency,
			:fill_order_id, :fill_product_id, :fill_side, :fill_price, :fill_commission, :idem,
			:network_hash, :network_name, :network_fee,
			:to_resource, :to_address, :to_id, :to_email, :to_name,
			:from_resource, :from_address, :from_id, :from_email, :from_name,
			:buy_id, :buy_fee, :buy_total, :buy_subtotal, :buy_payment_method_name,
			:sell_id, :sell_fee, :sell_total, :sell_subtotal, :sell_payment_method_name
		)
	`)
	if err != nil {
		log.Fatalf("preparing statement: %v", err)
	}
	defer stmt.Close()

	// Fetch all transactions from beginning (order=asc means oldest first)
	log.Printf("fetching full transaction history for %s...", currency)
	var cursor string
	total := 0

	for {
		resp, err := client.ListV2Transactions(account.ID, cursor, 100)
		if err != nil {
			log.Fatalf("fetching transactions: %v", err)
		}

		for _, t := range resp.Data {
			total++

			// Network fields
			var networkHash, networkName, networkFee string
			if t.Network != nil {
				networkHash = t.Network.Hash
				networkName = t.Network.NetworkName
				if t.Network.TransactionFee != nil {
					networkFee = t.Network.TransactionFee.Amount
				}
			}

			// To party fields
			var toResource, toAddress, toID, toEmail, toName string
			if t.To != nil {
				toResource = t.To.Resource
				toAddress = t.To.Address
				toID = t.To.ID
				toEmail = t.To.Email
				toName = t.To.Name
			}

			// From party fields
			var fromResource, fromAddress, fromID, fromEmail, fromName string
			if t.From != nil {
				fromResource = t.From.Resource
				fromAddress = t.From.Address
				fromID = t.From.ID
				fromEmail = t.From.Email
				fromName = t.From.Name
			}

			// Buy fields
			var buyID, buyFee, buyTotal, buySubtotal, buyPaymentMethodName string
			if t.Buy != nil {
				buyID = t.Buy.ID
				buyFee = t.Buy.Fee.Amount
				buyTotal = t.Buy.Total.Amount
				buySubtotal = t.Buy.Subtotal.Amount
				buyPaymentMethodName = t.Buy.PaymentMethodName
			}

			// Sell fields
			var sellID, sellFee, sellTotal, sellSubtotal, sellPaymentMethodName string
			if t.Sell != nil {
				sellID = t.Sell.ID
				sellFee = t.Sell.Fee.Amount
				sellTotal = t.Sell.Total.Amount
				sellSubtotal = t.Sell.Subtotal.Amount
				sellPaymentMethodName = t.Sell.PaymentMethodName
			}

			// Fill fields
			var fillOrderID, fillProductID, fillSide, fillPrice, fillCommission string
			if t.AdvancedTradeFill != nil {
				fillOrderID = t.AdvancedTradeFill.OrderID
				fillProductID = t.AdvancedTradeFill.ProductID
				fillSide = t.AdvancedTradeFill.OrderSide
				fillPrice = t.AdvancedTradeFill.FillPrice
				fillCommission = t.AdvancedTradeFill.Commission
			}

			// Parse timestamps
			createdAt := parseRFC3339Micros(t.CreatedAt)
			var updatedAt *int64
			if t.UpdatedAt != "" {
				u := parseRFC3339Micros(t.UpdatedAt)
				updatedAt = &u
			}

			_, err := stmt.Exec(
				sql.Named("id", t.ID),
				sql.Named("account_id", account.ID),
				sql.Named("type", t.Type),
				sql.Named("status", t.Status),
				sql.Named("created_at", createdAt),
				sql.Named("updated_at", updatedAt),
				sql.Named("amount", t.Amount.Amount),
				sql.Named("currency", t.Amount.Currency),
				sql.Named("native_amount", t.NativeAmount.Amount),
				sql.Named("native_currency", t.NativeAmount.Currency),
				sql.Named("fill_order_id", fillOrderID),
				sql.Named("fill_product_id", fillProductID),
				sql.Named("fill_side", fillSide),
				sql.Named("fill_price", fillPrice),
				sql.Named("fill_commission", fillCommission),
				sql.Named("idem", t.Idem),
				sql.Named("network_hash", networkHash),
				sql.Named("network_name", networkName),
				sql.Named("network_fee", networkFee),
				sql.Named("to_resource", toResource),
				sql.Named("to_address", toAddress),
				sql.Named("to_id", toID),
				sql.Named("to_email", toEmail),
				sql.Named("to_name", toName),
				sql.Named("from_resource", fromResource),
				sql.Named("from_address", fromAddress),
				sql.Named("from_id", fromID),
				sql.Named("from_email", fromEmail),
				sql.Named("from_name", fromName),
				sql.Named("buy_id", buyID),
				sql.Named("buy_fee", buyFee),
				sql.Named("buy_total", buyTotal),
				sql.Named("buy_subtotal", buySubtotal),
				sql.Named("buy_payment_method_name", buyPaymentMethodName),
				sql.Named("sell_id", sellID),
				sql.Named("sell_fee", sellFee),
				sql.Named("sell_total", sellTotal),
				sql.Named("sell_subtotal", sellSubtotal),
				sql.Named("sell_payment_method_name", sellPaymentMethodName),
			)
			if err != nil {
				log.Fatalf("inserting transaction %s: %v", t.ID, err)
			}
		}

		if resp.Pagination.NextStartingAfter == nil || len(resp.Data) == 0 {
			break
		}
		log.Printf("resynced %d transactions...", total)
		cursor = *resp.Pagination.NextStartingAfter
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("committing transaction: %v", err)
	}

	log.Printf("resync complete: %d transactions inserted", total)
}

func parseRFC3339Micros(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.UnixMicro()
}
