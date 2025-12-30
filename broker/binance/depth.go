package binance

import (
	"fmt"
)

// DepthSnapshot is the response from the depth REST endpoint.
type DepthSnapshot struct {
	LastUpdateID int64       `json:"lastUpdateId"`
	Bids         [][2]string `json:"bids"` // [[price, qty], ...]
	Asks         [][2]string `json:"asks"` // [[price, qty], ...]
}

// GetDepth fetches the order book snapshot for a symbol.
func (c *Client) GetDepth(symbol string, limit int) (*DepthSnapshot, error) {
	path := "/api/v3/depth?symbol=" + symbol
	if limit >= 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}
	var snapshot *DepthSnapshot
	if err := c.GetJSON(path, &snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}
