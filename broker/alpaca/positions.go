package alpaca

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Position represents an open position.
type Position struct {
	AssetID                string `json:"asset_id"`    // e.g. 904837e3-3b76-4c7d-8534-8e7a3e0a6ed1
	Symbol                 string `json:"symbol"`      // e.g. AAPL, BTCUSD
	Exchange               string `json:"exchange"`    // e.g. NASDAQ, CRYPTO
	AssetClass             string `json:"asset_class"` // e.g. us_equity, us_option, crypto
	AssetMarginable        bool   `json:"asset_marginable"`
	Qty                    string `json:"qty"`           // the number of shares
	QtyAvailable           string `json:"qty_available"` // total number of shares available, minus open orders / locked for options covered call
	Side                   string `json:"side"`          // e.g. long, short
	MarketValue            string `json:"market_value"`  // total dollar amount of position
	AvgEntryPrice          string `json:"avg_entry_price"`
	CostBasis              string `json:"cost_basis"`               // total cost basis in dollars
	UnrealizedPL           string `json:"unrealized_pl"`            // in dollars
	UnrealizedPLPC         string `json:"unrealized_plpc"`          // percent (by a factor of 1)
	UnrealizedIntradayPL   string `json:"unrealized_intraday_pl"`   // in dollars
	UnrealizedIntradayPLPC string `json:"unrealized_intraday_plpc"` // percent (by a factor of 1)
	CurrentPrice           string `json:"current_price"`
	LastDayPrice           string `json:"lastday_price"`
	ChangeToday            string `json:"change_today"` // percent change from last day price (by a factor of 1)
}

// GetPositions retrieves all open positions.
func (c *Client) GetPositions() ([]Position, error) {
	resp, err := c.Get("/v2/positions")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result []Position
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return result, nil
}
