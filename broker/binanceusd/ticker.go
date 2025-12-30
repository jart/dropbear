package binanceusd

type Ticker struct {
	Symbol             string `json:"symbol"`             // e.g. "BTCUSDT"
	PriceChange        string `json:"priceChange"`        // e.g. "1346.20"
	PriceChangePercent string `json:"priceChangePercent"` // e.g. "1.540"
	WeightedAvgPrice   string `json:"weightedAvgPrice"`   // e.g. "88323.23"
	LastPrice          string `json:"lastPrice"`          // e.g. "88770.30"
	LastQty            string `json:"lastQty"`            // e.g. "0.003"
	OpenPrice          string `json:"openPrice"`          // e.g. "87424.10"
	HighPrice          string `json:"highPrice"`          // e.g. "89557.60"
	LowPrice           string `json:"lowPrice"`           // e.g. "86824.80"
	Volume             string `json:"volume"`             // e.g. "111434.432"
	QuoteVolume        string `json:"quoteVolume"`        // e.g. "9842248907.57"
	OpenTime           int64  `json:"openTime"`           // e.g. 1766657400000
	CloseTime          int64  `json:"closeTime"`          // e.g. 1766743837072
	FirstID            int64  `json:"firstId"`            // e.g. 7049598779
	LastID             int64  `json:"lastId"`             // e.g. 7052419005
	Count              int64  `json:"count"`              // e.g. 2813812
}

// GetTicker retrieves 24-hour rolling price change statistics for symbol.
// https://developers.binance.com/docs/derivatives/usds-margined-futures/market-data/rest-api/24hr-Ticker-Price-Change-Statistics
func (c *Client) GetTicker(symbol string) (*Ticker, error) {
	uri := "/fapi/v1/ticker/24hr?symbol=" + symbol
	var ticker *Ticker
	if err := c.GetJSON(uri, &ticker); err != nil {
		return nil, err
	}
	return ticker, nil
}

// GetTickers retrieves 24-hour rolling price change statistics for all symbols.
// https://developers.binance.com/docs/derivatives/usds-margined-futures/market-data/rest-api/24hr-Ticker-Price-Change-Statistics
func (c *Client) GetTickers() ([]*Ticker, error) {
	uri := "/fapi/v1/ticker/24hr"
	var tickers []*Ticker
	if err := c.GetJSON(uri, &tickers); err != nil {
		return nil, err
	}
	return tickers, nil
}
