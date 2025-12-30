package binance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type RateLimit struct {
	RateLimitType string `json:"rateLimitType"`
	Interval      string `json:"interval"`
	IntervalNum   int    `json:"intervalNum"`
	Limit         int    `json:"limit"`
}

type Filter struct {
	FilterType            string `json:"filterType"`                      // e.g. PRICE_FILTER
	MinPrice              string `json:"minPrice,omitempty"`              // e.g. "0.00000100"
	MaxPrice              string `json:"maxPrice,omitempty"`              // e.g. "100000.00000000"
	TickSize              string `json:"tickSize,omitempty"`              // e.g. "0.00000100"
	MinQty                string `json:"minQty,omitempty"`                // e.g. "0.00010000"
	MaxQty                string `json:"maxQty,omitempty"`                // e.g. "100000.00000000"
	StepSize              string `json:"stepSize,omitempty"`              // e.g. "0.00010000"
	MinNotional           string `json:"minNotional,omitempty"`           // e.g. "5.00000000"
	MaxNotional           string `json:"maxNotional,omitempty"`           // e.g. "9000000.00000000"
	ApplyToMarket         bool   `json:"applyToMarket,omitempty"`         // e.g. true
	AvgPriceMins          int    `json:"avgPriceMins,omitempty"`          // e.g. 5
	Limit                 int    `json:"limit,omitempty"`                 // e.g. 200
	MaxNumOrders          int    `json:"maxNumOrders,omitempty"`          // e.g. 200
	MaxNumAlgoOrders      int    `json:"maxNumAlgoOrders,omitempty"`      // e.g. 5
	MinTrailingAboveDelta int    `json:"minTrailingAboveDelta,omitempty"` // e.g. "0.00000000"
	MaxTrailingAboveDelta int    `json:"maxTrailingAboveDelta,omitempty"` // e.g. "100000.00000000"
	MinTrailingBelowDelta int    `json:"minTrailingBelowDelta,omitempty"` // e.g. "0.00000000"
	MaxTrailingBelowDelta int    `json:"maxTrailingBelowDelta,omitempty"` // e.g. "100000.00000000"
	BidMultiplierUp       string `json:"bidMultiplierUp,omitempty"`       // e.g. "1.10000000"
	BidMultiplierDown     string `json:"bidMultiplierDown,omitempty"`     // e.g. "0.90000000"
	AskMultiplierUp       string `json:"askMultiplierUp,omitempty"`       // e.g. "1.10000000"
	AskMultiplierDown     string `json:"askMultiplierDown,omitempty"`     // e.g. "0.90000000"
	MaxTrailingAbovePct   string `json:"maxTrailingAbovePct,omitempty"`   // e.g. "5.00000000"
	MaxTrailingBelowPct   string `json:"maxTrailingBelowPct,omitempty"`   // e.g. "5.00000000"
}

type Symbol struct {
	Symbol                          string    `json:"symbol"`                                    // e.g. BTCFDUSD
	Status                          string    `json:"status"`                                    // e.g. TRADING
	BaseAsset                       string    `json:"baseAsset"`                                 // e.g. BTC
	BaseAssetPrecision              int       `json:"baseAssetPrecision"`                        // e.g. 8
	QuoteAsset                      string    `json:"quoteAsset"`                                // e.g. FDUSD
	QuoteAssetPrecision             int       `json:"quoteAssetPrecision"`                       // e.g. 8
	BaseCommissionPrecision         int       `json:"baseCommissionPrecision"`                   // e.g. 8
	QuoteCommissionPrecision        int       `json:"quoteCommissionPrecision"`                  // e.g. 8
	IcebergAllowed                  bool      `json:"icebergAllowed"`                            // e.g. true
	OrderTypes                      []string  `json:"orderTypes"`                                // e.g. ["LIMIT", "LIMIT_MAKER", "MARKET"]
	OCOAllowed                      bool      `json:"ocoAllowed"`                                // e.g. true
	OTOAllowed                      bool      `json:"otoAllowed"`                                // e.g. true
	OPOAllowed                      bool      `json:"opoAllowed"`                                // e.g. true
	QuoteOrderQtyMarketAllowed      bool      `json:"quoteOrderQtyMarketAllowed"`                // e.g. true
	AllowTrailingStop               bool      `json:"allowTrailingStop"`                         // e.g. true
	CancelReplaceAllowed            bool      `json:"cancelReplaceAllowed"`                      // e.g. true
	AmendAllowed                    bool      `json:"amendAllowed"`                              // e.g. true
	PegInstructionsAllowed          bool      `json:"pegInstructionsAllowed"`                    // e.g. true
	IsSpotTradingAllowed            bool      `json:"isSpotTradingAllowed"`                      // e.g. true
	IsMarginTradingAllowed          bool      `json:"isMarginTradingAllowed"`                    // e.g. true
	Filters                         []*Filter `json:"filters"`                                   // e.g. [...]
	DefaultSelfTradePreventionMode  string    `json:"defaultSelfTradePreventionMode,omitempty"`  // e.g. "EXPIRE_MAKER"
	AllowedSelfTradePreventionModes []string  `json:"allowedSelfTradePreventionModes,omitempty"` // e.g. ["EXPIRE_TAKER", "EXPIRE_MAKER", "EXPIRE_BOTH", "DECREMENT"]
}

type ExchangeInfo struct {
	Timezone   string      `json:"timezone"`   // e.g. UTC
	ServerTime int64       `json:"serverTime"` // unix milliseconds
	RateLimits []RateLimit `json:"rateLimits"`
	Symbols    []*Symbol   `json:"symbols"`
}

// GetExchangeInfo retrieves current exchange trading rules and symbol information.
func (c *Client) GetExchangeInfo() (*ExchangeInfo, error) {
	return c.getExchangeInfo("")
}

// GetSymbol retrieves symbol information for a specific trading pair.
func (c *Client) GetSymbol(symbol string) (*Symbol, error) {
	res, err := c.getExchangeInfo(symbol)
	if err != nil {
		return nil, err
	}
	// api will return an error if the symbol is invalid
	return res.Symbols[0], nil
}

// https://developers.binance.com/docs/binance-spot-api-docs/rest-api/general-endpoints#exchange-information
func (c *Client) getExchangeInfo(symbol string) (*ExchangeInfo, error) {
	uri := "/api/v3/exchangeInfo"
	if symbol != "" {
		uri += "?symbol=" + symbol
	}
	resp, err := c.Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch exchangeInfo: %d %s", resp.StatusCode, string(body))
	}
	var res ExchangeInfo
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode exchangeInfo: %w", err)
	}
	return &res, nil
}
