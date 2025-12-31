package coinbase

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

//go:embed transactions.sql
var transactionsSchema string

// TransactionsSchema returns the SQL schema for the coinbase_transactions table.
func TransactionsSchema() string {
	return transactionsSchema
}

// V2Transaction represents a Coinbase v2 API transaction.
// Transaction types include: send, receive, buy, sell, trade, transfer,
// advanced_trade_fill, fiat_deposit, fiat_withdrawal, and others.
type V2Transaction struct {
	ID           string `json:"id"`
	Type         string `json:"type"`   // send, buy, sell, advanced_trade_fill, etc.
	Status       string `json:"status"` // pending, completed, failed, etc.
	Resource     string `json:"resource"`
	ResourcePath string `json:"resource_path"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at,omitempty"`

	// Amount is the crypto amount (positive = credit, negative = debit)
	Amount V2CurrencyAmount `json:"amount"`

	// NativeAmount is the fiat equivalent at time of transaction
	NativeAmount V2CurrencyAmount `json:"native_amount"`

	// Network details for on-chain transactions (send/receive)
	Network *V2TransactionNetwork `json:"network,omitempty"`

	// To is the destination for send transactions
	To *V2TransactionParty `json:"to,omitempty"`

	// From is the source for receive transactions
	From *V2TransactionParty `json:"from,omitempty"`

	// Buy details for type="buy" (fiat purchase)
	Buy *V2TransactionFund `json:"buy,omitempty"`

	// Sell details for type="sell" (fiat sale)
	Sell *V2TransactionFund `json:"sell,omitempty"`

	// AdvancedTradeFill details for type="advanced_trade_fill"
	AdvancedTradeFill *V2TransactionFill `json:"advanced_trade_fill,omitempty"`

	// Trade details for type="trade" (crypto-to-crypto)
	Trade *V2TransactionTrade `json:"trade,omitempty"`

	// Idem is the idempotency key if one was provided
	Idem string `json:"idem,omitempty"`

	// Description is an optional user-provided memo
	Description string `json:"description,omitempty"`
}

// V2CurrencyAmount represents an amount with its currency.
type V2CurrencyAmount struct {
	Amount   string `json:"amount"`   // decimal string, may be negative
	Currency string `json:"currency"` // e.g. "BTC", "USD"
}

// V2TransactionNetwork contains on-chain transaction details.
type V2TransactionNetwork struct {
	Status         string            `json:"status"`       // pending, unconfirmed, confirmed
	Hash           string            `json:"hash"`         // blockchain transaction hash
	NetworkName    string            `json:"network_name"` // e.g. "bitcoin", "ethereum"
	TransactionFee *V2CurrencyAmount `json:"transaction_fee,omitempty"`
	Confirmations  int               `json:"confirmations,omitempty"`
}

// V2TransactionParty represents a sender or recipient.
type V2TransactionParty struct {
	Resource string `json:"resource"`          // "address", "user", "account"
	Address  string `json:"address,omitempty"` // blockchain address
	ID       string `json:"id,omitempty"`      // Coinbase user/account ID
	Email    string `json:"email,omitempty"`
	Name     string `json:"name,omitempty"`
}

// V2TransactionFund contains details for deposits/withdrawls (buy/sell types).
type V2TransactionFund struct {
	ID                string           `json:"id"`
	PaymentMethodName string           `json:"payment_method_name"`
	Fee               V2CurrencyAmount `json:"fee"`
	Subtotal          V2CurrencyAmount `json:"subtotal"`
	Total             V2CurrencyAmount `json:"total"`
}

// V2TransactionFill contains details for advanced trade fills.
type V2TransactionFill struct {
	OrderID    string `json:"order_id"`
	ProductID  string `json:"product_id"` // e.g. "BTC-USD"
	OrderSide  string `json:"order_side"` // "buy" or "sell"
	FillPrice  string `json:"fill_price"` // price per unit
	Commission string `json:"commission"` // fee in quote currency
}

// V2TransactionTrade contains details for crypto-to-crypto trades.
type V2TransactionTrade struct {
	ID           string           `json:"id"`
	UnitPrice    V2CurrencyAmount `json:"unit_price"`
	Fee          V2CurrencyAmount `json:"fee"`
	Subtotal     V2CurrencyAmount `json:"subtotal"`
	FiatAmount   V2CurrencyAmount `json:"fiat_amount"`
	InputAmount  V2CurrencyAmount `json:"input_amount"`
	OutputAmount V2CurrencyAmount `json:"output_amount"`
}

// V2TransactionsResponse is the paginated response from the transactions endpoint.
type V2TransactionsResponse struct {
	Data       []V2Transaction          `json:"data"`
	Pagination V2TransactionsPagination `json:"pagination"`
}

// V2TransactionsPagination contains pagination info for transactions.
type V2TransactionsPagination struct {
	EndingBefore         *string `json:"ending_before"`
	StartingAfter        *string `json:"starting_after"`
	PreviousEndingBefore *string `json:"previous_ending_before"`
	NextStartingAfter    *string `json:"next_starting_after"`
	Limit                int     `json:"limit"`
	Order                string  `json:"order"` // "asc" or "desc"
	PreviousURI          *string `json:"previous_uri"`
	NextURI              *string `json:"next_uri"`
}

// ListV2Transactions fetches transactions for an account.
// accountID is the v2 account UUID.
// startingAfter is an optional cursor for pagination (transaction ID).
// limit is the max number of results (default 25, max 100).
// https://docs.cdp.coinbase.com/coinbase-business/track-apis/transactions#list-transactions
func (c *Client) ListV2Transactions(accountID string, startingAfter string, limit int) (*V2TransactionsResponse, error) {
	if limit <= 0 {
		limit = 100
	}
	path := fmt.Sprintf("/v2/accounts/%s/transactions?limit=%d&order=asc", accountID, limit)
	if startingAfter != "" {
		path += "&starting_after=" + startingAfter
	}
	c.rateLimiter.Get()
	resp, err := c.Get(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("list transactions error %d: %s", resp.StatusCode, string(body))
	}
	var result V2TransactionsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding transactions: %w", err)
	}
	return &result, nil
}

// IsCredit returns true if this transaction adds to the account balance.
func (t *V2Transaction) IsCredit() bool {
	if len(t.Amount.Amount) == 0 {
		return false
	}
	return t.Amount.Amount[0] != '-'
}

// IsDebit returns true if this transaction subtracts from the account balance.
func (t *V2Transaction) IsDebit() bool {
	if len(t.Amount.Amount) == 0 {
		return false
	}
	return t.Amount.Amount[0] == '-'
}

// TransactionSummary is the response from the transaction summary endpoint.
type TransactionSummary struct {
	TotalFees               json.Number    `json:"total_fees"`
	FeeTier                 FeeTierDetails `json:"fee_tier"`
	AdvancedTradeOnlyVolume json.Number    `json:"advanced_trade_only_volume"`
	AdvancedTradeOnlyFees   json.Number    `json:"advanced_trade_only_fees"`
	CoinbaseProVolume       json.Number    `json:"coinbase_pro_volume"`
	CoinbaseProFees         json.Number    `json:"coinbase_pro_fees"`
	TotalBalance            string         `json:"total_balance"`
}

// FeeTierDetails contains fee tier information from the transaction summary.
type FeeTierDetails struct {
	PricingTier  string `json:"pricing_tier"`
	TakerFeeRate string `json:"taker_fee_rate"`
	MakerFeeRate string `json:"maker_fee_rate"`
}

// GetTransactionSummary returns fee tier and volume summary.
// https://docs.cdp.coinbase.com/api-reference/advanced-trade-api/rest-api/fees/get-transaction-summary
func (c *Client) GetTransactionSummary() (*TransactionSummary, error) {
	c.rateLimiter.Get()
	resp, err := c.Get("/api/v3/brokerage/transaction_summary")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get transaction summary failed %d: %s", resp.StatusCode, string(respBody))
	}
	var result TransactionSummary
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding transaction summary: %w", err)
	}
	return &result, nil
}

// parseRFC3339Micros parses an RFC3339 timestamp string to unix microseconds.
func parseRFC3339Micros(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.UnixMicro()
}

// SyncTransactions synchronizes v2 transactions from the Coinbase API to the local database.
// It fetches transactions for the specified currency incrementally from the last synced transaction.
func (c *Client) SyncTransactions(currency string) error {

	// create transactions table if it doesn't exist
	_, err := c.db.Exec(transactionsSchema)
	if err != nil {
		return fmt.Errorf("creating transactions table: %w", err)
	}

	// get account id for currency
	accounts, err := c.GetAccounts()
	if err != nil {
		return fmt.Errorf("getting account for %s: %w", currency, err)
	}
	var accountID string
	for _, account := range accounts {
		if account.Currency == currency {
			accountID = account.UUID
			break
		}
	}
	if accountID == "" {
		return nil // there isn't any of this currency in auth key's portfolio
	}

	// Get cursor for incremental sync.
	// We start from the 10th most recent transaction to catch any that Coinbase
	// may have committed out of order. We also check for any unconfirmed on-chain
	// transactions and start from whichever is earlier, to ensure we re-sync
	// transactions that may have updated (e.g., confirmations).
	var cursor string
	var cursorCreatedAt int64

	// Get 10th most recent transaction
	err = c.db.QueryRow(`
		SELECT COALESCE(id, ''), COALESCE(created_at, 0)
		FROM coinbase_transactions
		WHERE account_id = :account_id
		ORDER BY created_at DESC
		LIMIT 1 OFFSET 9
	`, sql.Named("account_id", accountID)).Scan(&cursor, &cursorCreatedAt)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("getting recent transaction: %w", err)
	}

	// Check for earliest unconfirmed on-chain transaction
	var unconfirmedCursor string
	var unconfirmedCreatedAt int64
	err = c.db.QueryRow(`
		SELECT id, created_at
		FROM coinbase_transactions
		WHERE account_id = :account_id
		  AND network_hash IS NOT NULL
		  AND network_hash != ''
		  AND status != 'completed'
		ORDER BY created_at ASC
		LIMIT 1
	`, sql.Named("account_id", accountID)).Scan(&unconfirmedCursor, &unconfirmedCreatedAt)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("getting unconfirmed transaction: %w", err)
	}

	// Use whichever cursor is earlier
	if unconfirmedCursor != "" && (cursor == "" || unconfirmedCreatedAt < cursorCreatedAt) {
		cursor = unconfirmedCursor
	}

	// Start transaction for atomic insert
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare insert statement with named parameters
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
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	total := 0
	for {
		resp, err := c.ListV2Transactions(accountID, cursor, 100)
		if err != nil {
			return fmt.Errorf("fetching transactions: %w", err)
		}
		for _, t := range resp.Data {
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

			result, err := stmt.Exec(
				sql.Named("id", t.ID),
				sql.Named("account_id", accountID),
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
				return fmt.Errorf("inserting transaction %s: %w", t.ID, err)
			}
			if n, _ := result.RowsAffected(); n > 0 {
				total++
			}
		}
		if resp.Pagination.NextStartingAfter == nil || len(resp.Data) == 0 {
			break
		}
		log.Printf("synced %d %s transactions...", total, currency)
		cursor = *resp.Pagination.NextStartingAfter
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	if total > 0 {
		log.Printf("synced %d %s transactions", total, currency)
	}
	return nil
}
