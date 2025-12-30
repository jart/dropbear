package binanceusd

import (
	"fmt"
)

// DepthSnapshot is the response from the depth REST endpoint.
type DepthSnapshot struct {
	LastUpdateID      int64       `json:"lastUpdateId"`
	MessageOutputTime int64       `json:"E"`
	TransactionTime   int64       `json:"T"`
	Bids              [][2]string `json:"bids"` // [[price, qty], ...]
	Asks              [][2]string `json:"asks"` // [[price, qty], ...]
}

// GetDepthSnapshot fetches the order book snapshot for a symbol.
// The limit parameter defaults to 100 and has a maximum of 1000.
// https://developers.binance.com/docs/derivatives/usds-margined-futures/market-data/rest-api/Order-Book
func (c *Client) GetDepth(symbol string, limit int) (*DepthSnapshot, error) {
	path := "/fapi/v1/depth?symbol=" + symbol
	if limit >= 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}
	var snapshot *DepthSnapshot
	if err := c.GetJSON(path, &snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}
