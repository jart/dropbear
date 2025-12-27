package coinbase

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Product represents a currency pair.
type Product struct {
	ProductID          string `json:"product_id"`           // e.g. BTC-USD
	BaseDisplaySymbol  string `json:"base_display_symbol"`  // e.g. BTC
	QuoteDisplaySymbol string `json:"quote_display_symbol"` // e.g. USD
	Status             string `json:"status"`               // e.g. online
	Price              string `json:"price"`
	Volume24h          string `json:"volume_24h"`
	BaseIncrement      string `json:"base_increment"`
	QuoteIncrement     string `json:"quote_increment"`
	QuoteMinSize       string `json:"quote_min_size"`
	QuoteMaxSize       string `json:"quote_max_size"`
	BaseMinSize        string `json:"base_min_size"`
	BaseMaxSize        string `json:"base_max_size"`
	New                bool   `json:"new"`
	IsDisabled         bool   `json:"is_disabled"`
	CancelOnly         bool   `json:"cancel_only"`
	LimitOnly          bool   `json:"limit_only"`
	PostOnly           bool   `json:"post_only"`
	TradingDisabled    bool   `json:"trading_disabled"`
	AuctionMode        bool   `json:"auction_mode"`
	IsAlphaTesting     bool   `json:"is_alpha_testing"`
}

// ProductsResponse is the response from the products endpoint.
type ProductsResponse struct {
	Products []Product `json:"products"`
}

// GetProducts retrieves all available trading products.
func (c *Client) GetProducts() (*ProductsResponse, error) {
	c.rateLimiter.Get()
	resp, err := c.Get("/api/v3/brokerage/products")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result ProductsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// GetProduct retrieves information about a tradable pair on Coinbase.
func (c *Client) GetProduct(productID string) (*Product, error) {
	c.rateLimiter.Get()
	resp, err := c.Get("/api/v3/brokerage/products/" + productID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result Product
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
