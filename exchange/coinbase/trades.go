package coinbase

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"encoding/json"
	"fmt"
	"io"
)

// GetTrades fetches historical trades for a product.
// limit defaults to 100 and its maximum value is 1000.
// Returns trades in chronological order (oldest first).
func (c *Client) GetTrades(productID string, start, end clocky.Time, limit int) ([]*ds.Trade, error) {
	url := "/api/v3/brokerage/market/products/" + productID + "/ticks"
	if start > 0 {
		url = fmt.Sprintf("%s&start=%d", url, start.Unix())
	}
	if end > 0 {
		url = fmt.Sprintf("%s&end=%d", url, end.Unix())
	}
	if limit > 0 {
		url = fmt.Sprintf("%s&limit=%d", url, limit)
	}
	c.rateLimiter.Get()
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("trades request failed %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		BestBid string `json:"best_bid"`
		BestAsk string `json:"best_ask"`
		Trades  []struct {
			TradeID   string `json:"trade_id"`   // e.g. "926851416"
			ProductID string `json:"product_id"` // e.g. "BTC-USD"
			Price     string `json:"price"`      // e.g. "87818.02"
			Size      string `json:"size"`       // e.g. "0.00236744"
			Time      string `json:"time"`       // e.g. "2025-12-25T22:14:18.406361Z"
			Side      string `json:"side"`       // what maker did, i.e. "BUY" or "SELL"
			Bid       string `json:"bid"`        // always "" it seems
			Ask       string `json:"ask"`        // always "" it seems
			Exchange  string `json:"exchange"`   // always "coinbase" it seems
		} `json:"trades"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	n := len(result.Trades)
	trades := make([]*ds.Trade, n)
	for i, trade := range result.Trades {
		trades[n-1-i] = &ds.Trade{
			Side:     ds.MustParseSide(trade.Side).Flip(),
			Time:     clocky.MustParseTime(trade.Time),
			Price:    decimal.Parse(trade.Price),
			Quantity: decimal.Parse(trade.Size),
		}
	}
	return trades, nil
}
