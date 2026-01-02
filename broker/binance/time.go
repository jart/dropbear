package binance

import (
	"dropbear/clocky"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GetTime retrieves the current server time.
func (c *Client) GetTime() (clocky.Time, error) {
	uri := "/api/v3/time"
	resp, err := c.Get(uri)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("failed to fetch ticker: %d %s", resp.StatusCode, string(body))
	}
	var ticker struct {
		ServerTime int64 `json:"serverTime"` // unix milliseconds
	}
	if err := json.NewDecoder(resp.Body).Decode(&ticker); err != nil {
		return 0, fmt.Errorf("failed to decode ticker: %w", err)
	}
	return clocky.Time(ticker.ServerTime * 1000), nil
}
