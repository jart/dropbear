package coinbase

import (
	"encoding/json"
	"fmt"
	"io"
)

// V2Account represents a Coinbase v2 API account.
type V2Account struct {
	ID               string `json:"id"`
	Name             string `json:"name"` // e.g. "Cash (USD)", "Bitcoin"
	Primary          bool   `json:"primary"`
	Type             string `json:"type"`          // e.g. crypto, wallet, fiat
	CreatedAt        string `json:"created_at"`    // e.g. 2025-12-01T11:48:06Z
	UpdatedAt        string `json:"updated_at"`    // e.g. 2025-12-01T11:48:06Z
	Resource         string `json:"resource"`      // e.g. account
	ResourcePath     string `json:"resource_path"` // e.g. /v2/accounts/$id
	AllowDeposits    bool   `json:"allow_deposits"`
	AllowWithdrawals bool   `json:"allow_withdrawals"`
	PortfolioID      string `json:"portfolio_id"`
	Balance          struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"` // e.g. BTC
	} `json:"balance"`
	Currency struct {
		AssetID  string `json:"asset_id"` // e.g. 3f6f5c2a-3d5b-5e3a-9f2d-123456789abc
		Code     string `json:"code"`     // e.g. BTC
		Name     string `json:"name"`     // e.g. Bitcoin
		Slug     string `json:"slug"`     // e.g. bitcoin
		Color    string `json:"color"`    // e.g. #f7931a
		Exponent int    `json:"exponent"` // e.g. 8
	} `json:"currency"`
}

// v2AccountsResponse is the response from the v2 accounts endpoint.
type v2AccountsResponse struct {
	Data       []*V2Account `json:"data"`
	Pagination struct {
		NextURI              string `json:"next_uri"`
		Order                string `json:"order"` // e.g. desc
		Limit                int    `json:"limit"` // e.g. 25
		PreviousEndingBefore string `json:"previous_ending_before"`
		PreviousUri          string `json:"previous_uri"`
		StartingAfter        string `json:"starting_after"`
		EndingBefore         string `json:"ending_before"`
		NextStartingAfter    string `json:"next_starting_after"`
	} `json:"pagination"`
}

// GetV2AccountByCurrencyCode finds the v2 account for a given currency code.
func (c *Client) GetV2AccountByCurrencyCode(symbol string) (*V2Account, error) {
	path := "/v2/accounts?limit=250" // 250 is the maximum limit
	for path != "" {
		resp, err := c.Get(path)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("v2 accounts error %d: %s", resp.StatusCode, string(body))
		}
		var result v2AccountsResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decoding v2 accounts: %w", err)
		}
		for _, acc := range result.Data {
			if acc != nil && acc.Currency.Code == symbol {
				return acc, nil
			}
		}
		path = result.Pagination.NextURI
	}
	return nil, fmt.Errorf("v2 account not found for %s", symbol)
}
