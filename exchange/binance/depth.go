package binance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DepthSnapshot is the response from the depth REST endpoint.
type DepthSnapshot struct {
	LastUpdateID int64      `json:"lastUpdateId"`
	Bids         [][]string `json:"bids"` // [[price, qty], ...]
	Asks         [][]string `json:"asks"` // [[price, qty], ...]
}

// GetDepthSnapshot fetches the order book snapshot for a symbol.
func (c *Client) GetDepthSnapshot(symbol string, limit int) (*DepthSnapshot, error) {
	path := fmt.Sprintf("/api/v3/depth?symbol=%s&limit=%d", symbol, limit)
	resp, err := c.Get(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("depth snapshot failed: %d %s", resp.StatusCode, string(body))
	}
	var snapshot DepthSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decoding depth snapshot: %w", err)
	}
	return &snapshot, nil
}
