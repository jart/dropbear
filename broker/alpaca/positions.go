package alpaca

import (
	"dropbear/decimal"
	"dropbear/netty"
)

// Position represents an open position.
type Position struct {
	AssetID                string          `json:"asset_id"`                 // e.g. 904837e3-3b76-4c7d-8534-8e7a3e0a6ed1
	Symbol                 string          `json:"symbol"`                   // e.g. AAPL, BTCUSD
	Exchange               Exchange        `json:"exchange"`                 // e.g. NASDAQ, CRYPTO
	AssetClass             AssetClass      `json:"asset_class"`              // e.g. us_equity, us_option, crypto
	Side                   PositionSide    `json:"side"`                     // e.g. long, short
	AssetMarginable        bool            `json:"asset_marginable"`         //
	Qty                    decimal.Decimal `json:"qty"`                      // the number of shares (negative if short)
	QtyAvailable           decimal.Decimal `json:"qty_available"`            // total number of shares available, minus open orders / locked for options covered call (negative if short)
	MarketValue            decimal.Decimal `json:"market_value"`             // total dollar amount of position (negative if short)
	AvgEntryPrice          decimal.Decimal `json:"avg_entry_price"`          // average entry price of this position (always positive)
	CostBasis              decimal.Decimal `json:"cost_basis"`               // total cost basis in dollars (negative if short)
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
	var result []Position
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.FastHTTPClient, "GET", "/v2/positions", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetPosition retrieves an open position.
func (c *Client) GetPosition(symbolOrAssetID string) (*Position, error) {
	var result Position
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.FastHTTPClient, "GET", "/v2/positions/"+symbolOrAssetID, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
