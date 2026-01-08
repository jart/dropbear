package alpaca

import (
	"dropbear/decimal"
	"dropbear/ds/symbol"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Position represents an open position.
type Position struct {
	AssetID                string          `json:"asset_id"`                 // e.g. 904837e3-3b76-4c7d-8534-8e7a3e0a6ed1
	Symbol                 symbol.Symbol   `json:"symbol"`                   // e.g. AAPL, BTCUSD
	Exchange               Exchange        `json:"exchange"`                 // e.g. NASDAQ, CRYPTO
	AssetClass             AssetClass      `json:"asset_class"`              // e.g. us_equity, us_option, crypto
	Side                   PositionSide    `json:"side"`                     // e.g. long, short
	AssetMarginable        bool            `json:"asset_marginable"`         //
	Qty                    decimal.Decimal `json:"qty"`                      // the number of shares
	QtyAvailable           decimal.Decimal `json:"qty_available"`            // total number of shares available, minus open orders / locked for options covered call
	MarketValue            decimal.Decimal `json:"market_value"`             // total dollar amount of position
	AvgEntryPrice          decimal.Decimal `json:"avg_entry_price"`          // average entry price of this position
	CostBasis              decimal.Decimal `json:"cost_basis"`               // total cost basis in dollars
	UnrealizedPL           decimal.Decimal `json:"unrealized_pl"`            // unrealized profit/loss in dollars
	UnrealizedPLPC         decimal.Decimal `json:"unrealized_plpc"`          // unrealized profit/loss percent (by a factor of 1)
	UnrealizedIntradayPL   decimal.Decimal `json:"unrealized_intraday_pl"`   // unrealized profit/loss in dollars for the day
	UnrealizedIntradayPLPC decimal.Decimal `json:"unrealized_intraday_plpc"` // unrealized profit/loss percent (by a factor of 1) for the day
	CurrentPrice           decimal.Decimal `json:"current_price"`            // current asset price per share
	LastDayPrice           decimal.Decimal `json:"lastday_price"`            // last day’s asset price per share based on the closing value of the last trading day
	ChangeToday            decimal.Decimal `json:"change_today"`             // percent change from last day price (by a factor of 1)
}

// GetPositions retrieves all open positions.
func (c *Client) GetPositions() ([]Position, error) {
	c.APITokenBucket.Get()
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

// GetPosition retrieves an open position.
func (c *Client) GetPosition(symbolOrAssetID fmt.Stringer) (*Position, error) {
	c.APITokenBucket.Get()
	resp, err := c.Get("/v2/positions/" + symbolOrAssetID.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result Position
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
