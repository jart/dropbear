package kraken

import (
	"net/url"
)

// AssetPair represents a tradable pair on Kraken.
type AssetPair struct {
	Altname           string      `json:"altname"`             // alternate pair name
	WSName            string      `json:"wsname"`              // WebSocket pair name
	AClassBase        string      `json:"aclass_base"`         // asset class of base component
	Base              string      `json:"base"`                // asset id of base component
	AClassQuote       string      `json:"aclass_quote"`        // asset class of quote component
	Quote             string      `json:"quote"`               // asset id of quote component
	Lot               string      `json:"lot"`                 // volume lot size (deprecated)
	CostDecimals      int         `json:"cost_decimals"`       // scaling decimal places for cost
	PairDecimals      int         `json:"pair_decimals"`       // scaling decimal places for pair (price precision)
	LotDecimals       int         `json:"lot_decimals"`        // scaling decimal places for volume
	LotMultiplier     int         `json:"lot_multiplier"`      // amount to multiply lot volume by to get currency volume
	LeverageBuy       []int       `json:"leverage_buy"`        // array of leverage amounts available when buying
	LeverageSell      []int       `json:"leverage_sell"`       // array of leverage amounts available when selling
	Fees              [][]float64 `json:"fees"`                // fee schedule array in [volume, percent fee] tuples
	FeesMaker         [][]float64 `json:"fees_maker"`          // maker fee schedule array in [volume, percent fee] tuples
	FeeVolumeCurrency string      `json:"fee_volume_currency"` // volume discount currency
	MarginCall        int         `json:"margin_call"`         // margin call level
	MarginStop        int         `json:"margin_stop"`         // stop-out/liquidation margin level
	OrderMin          string      `json:"ordermin"`            // minimum order size (in base currency)
	CostMin           string      `json:"costmin"`             // minimum order cost (in quote currency)
	TickSize          string      `json:"tick_size"`           // minimum price increment
	Status            string      `json:"status"`              // status of pair
}

// AssetPairsResponse is the response from the AssetPairs endpoint.
type AssetPairsResponse map[string]AssetPair

// GetAssetPairs retrieves all tradable asset pairs.
func (c *Client) GetAssetPairs() (AssetPairsResponse, error) {
	var result AssetPairsResponse
	if err := c.GetJSON("/0/public/AssetPairs", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetAssetPair retrieves information about a specific tradable pair.
func (c *Client) GetAssetPair(pair string) (*AssetPair, error) {
	c.restLimiter.Get()

	reqURL := "/0/public/AssetPairs?pair=" + url.QueryEscape(pair)
	resp, err := c.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result AssetPairsResponse
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}

	for _, v := range result {
		return &v, nil
	}
	return nil, nil
}

// Asset represents an asset on Kraken.
type Asset struct {
	AClass          string  `json:"aclass"`           // asset class
	Altname         string  `json:"altname"`          // alternate name
	Decimals        int     `json:"decimals"`         // scaling decimal places for record keeping
	DisplayDecimals int     `json:"display_decimals"` // scaling decimal places for output display
	CollateralValue float64 `json:"collateral_value"` // collateral value (for margin)
	Status          string  `json:"status"`           // status of asset
}

// AssetsResponse is the response from the Assets endpoint.
type AssetsResponse map[string]Asset

// GetAssets retrieves all assets.
func (c *Client) GetAssets() (AssetsResponse, error) {
	var result AssetsResponse
	if err := c.GetJSON("/0/public/Assets", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Ticker represents ticker information for a pair.
type Ticker struct {
	Ask            [3]string `json:"a"` // ask array [price, whole lot volume, lot volume]
	Bid            [3]string `json:"b"` // bid array [price, whole lot volume, lot volume]
	Last           [2]string `json:"c"` // last trade closed [price, lot volume]
	Volume         [2]string `json:"v"` // volume [today, last 24 hours]
	VWAP           [2]string `json:"p"` // volume weighted average price [today, last 24 hours]
	NumberOfTrades [2]int    `json:"t"` // number of trades [today, last 24 hours]
	Low            [2]string `json:"l"` // low [today, last 24 hours]
	High           [2]string `json:"h"` // high [today, last 24 hours]
	Open           string    `json:"o"` // today's opening price
}

// TickerResponse is the response from the Ticker endpoint.
type TickerResponse map[string]Ticker

// GetTicker retrieves ticker information for a pair.
func (c *Client) GetTicker(pair string) (*Ticker, error) {
	c.restLimiter.Get()

	reqURL := "/0/public/Ticker?pair=" + url.QueryEscape(pair)
	resp, err := c.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result TickerResponse
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}

	for _, v := range result {
		return &v, nil
	}
	return nil, nil
}
