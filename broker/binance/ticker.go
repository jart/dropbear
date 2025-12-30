package binance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Ticker struct {
	Symbol             string `json:"symbol"`             // e.g. "BTCUSDT"
	PriceChange        string `json:"priceChange"`        // e.g. "153.02000000"
	PriceChangePercent string `json:"priceChangePercent"` // e.g. "0.172"
	WeightedAvgPrice   string `json:"weightedAvgPrice"`   // e.g. "88946.36357837"
	OpenPrice          string `json:"openPrice"`          // e.g. "88974.98000000"
	HighPrice          string `json:"highPrice"`          // e.g. "89128.00000000"
	LowPrice           string `json:"lowPrice"`           // e.g. "88828.06000000"
	LastPrice          string `json:"lastPrice"`          // e.g. "89128.00000000"
	Volume             string `json:"volume"`             // e.g. "333.71620000"
	QuoteVolume        string `json:"quoteVolume"`        // e.g. "29682842.45719300"
	OpenTime           int64  `json:"openTime"`           // unix milliseconds
	CloseTime          int64  `json:"closeTime"`          // unix milliseconds
	FirstID            int64  `json:"firstId"`            // e.g. 5707077760
	LastID             int64  `json:"lastId"`             // e.g. 5707169969
	Count              int64  `json:"count"`              // e.g. 92210
}

// GetTicker retrieves rolling window price change statistics for symbol.
// windowSize defaults to "24h" (can be specified in minutes/hours/days).
// https://developers.binance.com/docs/binance-spot-api-docs/rest-api/market-data-endpoints#rolling-window-price-change-statistics
func (c *Client) GetTicker(symbol string, windowSize string) (*Ticker, error) {
	uri := "/api/v3/ticker?symbol=" + symbol
	if windowSize != "" {
		uri += "&windowSize=" + windowSize
	}
	resp, err := c.Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch ticker: %d %s", resp.StatusCode, string(body))
	}
	var ticker *Ticker
	if err := json.NewDecoder(resp.Body).Decode(&ticker); err != nil {
		return nil, fmt.Errorf("failed to decode ticker: %w", err)
	}
	return ticker, nil
}

// GetTickers24hr retrieves 24-hour rolling price change statistics for all symbols.
// https://developers.binance.com/docs/binance-spot-api-docs/rest-api/market-data-endpoints#24hr-ticker-price-change-statistics
func (c *Client) GetTickers24hr() ([]*Ticker, error) {
	uri := "/api/v3/ticker/24hr"
	var tickers []*Ticker
	if err := c.GetJSON(uri, &tickers); err != nil {
		return nil, err
	}
	return tickers, nil
}
