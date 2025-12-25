package coinbase

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Account represents a Coinbase account.
type Account struct {
	UUID             string         `json:"uuid"`
	Name             string         `json:"name"`
	Currency         string         `json:"currency"`
	AvailableBalance MonetaryAmount `json:"available_balance"`
	Hold             MonetaryAmount `json:"hold"`
}

// AccountsResponse is the response from the accounts endpoint.
type AccountsResponse struct {
	Accounts []Account `json:"accounts"`
	Cursor   string    `json:"cursor"`
	HasNext  bool      `json:"has_next"`
}

// GetAccounts retrieves all accounts with pagination.
func (c *Client) GetAccounts() (*AccountsResponse, error) {
	var allAccounts []Account
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
		var result AccountsResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		allAccounts = append(allAccounts, result.Accounts...)
		if !result.HasNext || result.Cursor == "" {
			break
		}
		cursor = result.Cursor
	}
	return &AccountsResponse{Accounts: allAccounts}, nil
}
