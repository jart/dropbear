package alpaca

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/ds"
	"dropbear/netty"
	"fmt"
	"strings"
)

// Snapshot represents a point-in-time view of a stock's market data.
// It includes the latest trade, latest quote, minute bar, daily bar,
// and previous daily bar.
type Snapshot struct {
	LatestTrade  *sip.Trade `json:"latestTrade"`
	LatestQuote  *Quote     `json:"latestQuote"`
	MinuteBar    *ds.Bar    `json:"minuteBar"`
	DailyBar     *ds.Bar    `json:"dailyBar"`
	PrevDailyBar *ds.Bar    `json:"prevDailyBar"`
}

// GetSnapshot fetches a comprehensive market data snapshot for a stock symbol.
// Returns the latest trade, latest quote, minute bar, daily bar, and previous daily bar.
// https://docs.alpaca.markets/reference/stocksnapshotsingle
func (c *Client) GetSnapshot(sym string) (*Snapshot, error) {
	url := fmt.Sprintf("https://%s/v2/stocks/%s/snapshot", DataHost, sym)
	var result Snapshot
	c.DataTokenBucket.Get()
	err := c.RequestJSON(netty.FastHTTPClient, "GET", url, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetSnapshots fetches snapshots for multiple stock symbols.
// Returns a map of symbol to snapshot.
// https://docs.alpaca.markets/reference/stocksnapshots
func (c *Client) GetSnapshots(symbols []string) (map[string]*Snapshot, error) {
	if len(symbols) == 0 {
		return nil, nil
	}
	var url strings.Builder
	fmt.Fprintf(&url, "https://%s/v2/stocks/snapshots?symbols=", DataHost)
	for i, sym := range symbols {
		if i > 0 {
			url.WriteString(",")
		}
		url.WriteString(sym)
	}
	var result map[string]*Snapshot
	c.DataTokenBucket.Get()
	err := c.RequestJSON(netty.BulkHttpClient, "GET", url.String(), nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
