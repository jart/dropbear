package coinbase

import (
	"dropbear/db"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

// Fill represents a single fill from the Coinbase API.
type Fill struct {
	EntryID            string `json:"entry_id"`
	TradeID            string `json:"trade_id"`
	OrderID            string `json:"order_id"`
	TradeTime          string `json:"trade_time"`
	TradeType          string `json:"trade_type"`
	Price              string `json:"price"`
	Size               string `json:"size"`
	Commission         string `json:"commission"`
	ProductID          string `json:"product_id"`
	SequenceTimestamp  string `json:"sequence_timestamp"`
	LiquidityIndicator string `json:"liquidity_indicator"` // MAKER, TAKER, or UNKNOWN_LIQUIDITY_INDICATOR
	SizeInQuote        bool   `json:"size_in_quote"`
	UserID             string `json:"user_id"`
	Side               string `json:"side"` // BUY or SELL
}

// ListFillsResponse is the response from listing fills.
type ListFillsResponse struct {
	Fills  []Fill `json:"fills"`
	Cursor string `json:"cursor"`
}

// ListFills returns fills matching the given filters.
// startTime and endTime should be RFC3339 format (e.g., "2024-01-01T00:00:00Z").
func (c *Client) ListFills(startTime, endTime, cursor string) (*ListFillsResponse, error) {
	params := url.Values{}
	if startTime != "" {
		params.Set("start_sequence_timestamp", startTime)
	}
	if endTime != "" {
		params.Set("end_sequence_timestamp", endTime)
	}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	params.Set("limit", "100")
	path := "/api/v3/brokerage/orders/historical/fills"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp, err := c.Get(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list fills failed %d: %s", resp.StatusCode, string(respBody))
	}
	var result ListFillsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding fills: %w", err)
	}
	return &result, nil
}

// SyncFills synchronizes fills from the Coinbase API to the local database.
// It fetches fills incrementally from the last sequence_timestamp forward.
func (c *Client) SyncFills() error {
	db := db.Get()
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS fills (
			entry_id TEXT PRIMARY KEY,
			trade_id TEXT NOT NULL,
			order_id TEXT NOT NULL,
			trade_time TEXT NOT NULL,
			trade_type TEXT NOT NULL,
			price TEXT NOT NULL,
			size TEXT NOT NULL,
			commission TEXT NOT NULL,
			product_id TEXT NOT NULL,
			sequence_timestamp TEXT NOT NULL,
			liquidity_indicator TEXT NOT NULL,
			size_in_quote INTEGER NOT NULL,
			user_id TEXT NOT NULL,
			side TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_fills_product_id ON fills(product_id);
		CREATE INDEX IF NOT EXISTS idx_fills_sequence_timestamp ON fills(sequence_timestamp);
		CREATE INDEX IF NOT EXISTS idx_fills_side ON fills(side);
	`)
	if err != nil {
		return fmt.Errorf("creating fills table in database: %w", err)
	}
	// Get the latest sequence_timestamp we have
	var startTime string
	err = db.QueryRow(`SELECT COALESCE(MAX(sequence_timestamp), '') FROM fills`).Scan(&startTime)
	if err != nil {
		return fmt.Errorf("getting last timestamp: %w", err)
	}
	// Prepare insert statement
	stmt, err := db.Prepare(`
		INSERT OR REPLACE INTO fills (
			entry_id, trade_id, order_id, trade_time, trade_type,
			price, size, commission, product_id, sequence_timestamp,
			liquidity_indicator, size_in_quote, user_id, side
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()
	cursor := ""
	total := 0
	for {
		resp, err := c.ListFills(startTime, "", cursor)
		if err != nil {
			return fmt.Errorf("fetching fills: %w", err)
		}
		for _, fill := range resp.Fills {
			total++
			sizeInQuote := 0
			if fill.SizeInQuote {
				sizeInQuote = 1
			}
			_, err := stmt.Exec(
				fill.EntryID,
				fill.TradeID,
				fill.OrderID,
				fill.TradeTime,
				fill.TradeType,
				fill.Price,
				fill.Size,
				fill.Commission,
				fill.ProductID,
				fill.SequenceTimestamp,
				fill.LiquidityIndicator,
				sizeInQuote,
				fill.UserID,
				fill.Side,
			)
			if err != nil {
				return fmt.Errorf("inserting fill: %w", err)
			}
		}
		if resp.Cursor == "" || len(resp.Fills) == 0 {
			break
		}
		log.Printf("synced %d fills...", total)
		cursor = resp.Cursor
	}
	if total > 0 {
		log.Printf("synced %d fills", total)
	}
	return nil
}
