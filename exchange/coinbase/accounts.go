package coinbase

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Account represents a Coinbase account.
type Account struct {
	UUID              string         `json:"uuid"`
	Name              string         `json:"name"`     // e.g. "BTC Wallet"
	Currency          string         `json:"currency"` // e.g. "BTC"
	Default           bool           `json:"default"`
	Active            bool           `json:"active"`
	Ready             bool           `json:"ready"`
	CreatedAt         string         `json:"created_at"` // e.g. "2025-12-17T23:31:39.664Z"
	UpdatedAt         string         `json:"updated_at"` // e.g. "2025-12-26T01:48:23.615578Z"
	DeletedAt         *string        `json:"deleted_at"` // e.g. null
	Type              string         `json:"type"`       // e.g. "ACCOUNT_TYPE_CRYPTO", "ACCOUNT_TYPE_FIAT", "ACCOUNT_TYPE_VAULT", "ACCOUNT_TYPE_PERP_FUTURES"
	RetailPortfolioID string         `json:"retail_portfolio_id"`
	Platform          string         `json:"platform"` // e.g. "ACCOUNT_PLATFORM_CONSUMER", "ACCOUNT_PLATFORM_INTX"
	AvailableBalance  MonetaryAmount `json:"available_balance"`
	Hold              MonetaryAmount `json:"hold"`
}

// GetAccounts retrieves all accounts with pagination.
func (c *Client) GetAccounts() ([]*Account, error) {
	var allAccounts []*Account
	cursor := ""
	for {
		path := "/api/v3/brokerage/accounts?limit=250"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		resp, err := c.Get(path)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
		}
		var result struct {
			Accounts []*Account `json:"accounts"`
			Cursor   string     `json:"cursor"`
			HasNext  bool       `json:"has_next"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		allAccounts = append(allAccounts, result.Accounts...)
		if !result.HasNext || result.Cursor == "" {
			break
		}
		cursor = result.Cursor
	}
	return allAccounts, nil
}
