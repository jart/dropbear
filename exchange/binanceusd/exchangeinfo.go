package binanceusd

import (
	"dropbear/exchange/binance"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Asset struct {
	Asset             string `json:"asset"`             // e.g. BTC
	MarginAvailable   bool   `json:"marginAvailable"`   // e.g. true
	AutoAssetExchange string `json:"autoAssetExchange"` // e.g. -0.10000000
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
	Symbol                string    `json:"symbol"`                // e.g. BTCUSDT
	Pair                  string    `json:"pair"`                  // e.g. BTCUSDT
	ContractType          string    `json:"contractType"`          // e.g. PERPETUAL
	DeliveryDate          int64     `json:"deliveryDate"`          // e.g. 4133404800000
	OnboardDate           int64     `json:"onboardDate"`           // e.g. 1569398400000
	Status                string    `json:"status"`                // e.g. TRADING
	MaintMarginPercent    string    `json:"maintMarginPercent"`    // e.g. "2.5000"
	RequiredMarginPercent string    `json:"requiredMarginPercent"` // e.g. "5.0000"
	BaseAsset             string    `json:"baseAsset"`             // e.g. BTC
	QuoteAsset            string    `json:"quoteAsset"`            // e.g. USDT
	MarginAsset           string    `json:"marginAsset"`           // e.g. USDT
	PricePrecision        int       `json:"pricePrecision"`        // e.g. 2
	QuantityPrecision     int       `json:"quantityPrecision"`     // e.g. 3
	BaseAssetPrecision    int       `json:"baseAssetPrecision"`    // e.g. 8
	QuotePrecision        int       `json:"quotePrecision"`        // e.g. 8
	UnderlyingType        string    `json:"underlyingType"`        // e.g. COIN
	UnderlyingSubType     []string  `json:"underlyingSubType"`     // e.g. [ "PoW" ]
	TriggerProtect        string    `json:"triggerProtect"`        // e.g. "0.0500"
	LiquidationFee        string    `json:"liquidationFee"`        // e.g. "0.012500"
	MarketTakeBound       string    `json:"marketTakeBound"`       // e.g. "0.05"
	MaxMoveOrderLimit     int       `json:"maxMoveOrderLimit"`     // e.g. 10000
	Filters               []*Filter `json:"filters"`               // e.g. [...]
	OrderTypes            []string  `json:"orderTypes"`            // e.g. ["LIMIT", "LIMIT_MAKER", "MARKET"]
	TimeInForce           []string  `json:"timeInForce"`           // e.g. ["GTC", "IOC", "FOK", "GTX", "GTD"]
	PermissionSets        []string  `json:"permissionSets"`        // e.g. ["GRID", "COPY", "DCA", "PSB"]
}

type ExchangeInfo struct {
	Timezone    string              `json:"timezone"`    // e.g. UTC
	ServerTime  int64               `json:"serverTime"`  // unix milliseconds
	FuturesType string              `json:"futuresType"` // e.g. "U_MARGINED"
	Assets      []*Asset            `json:"assets"`
	RateLimits  []binance.RateLimit `json:"rateLimits"`
	Symbols     []*Symbol           `json:"symbols"`
}

// GetExchangeInfo retrieves current exchange trading rules and symbol information.
// https://developers.binance.com/docs/derivatives/usds-margined-futures/market-data/rest-api/Exchange-Information
func (c *Client) GetExchangeInfo() (*ExchangeInfo, error) {
	uri := "/fapi/v1/exchangeInfo"
	resp, err := c.Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch exchange info: %d %s", resp.StatusCode, string(body))
	}
	var res ExchangeInfo
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode exchange info: %w", err)
	}
	return &res, nil
}
