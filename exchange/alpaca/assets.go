package alpaca

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"dropbear/decimal"
	"dropbear/ds"
)

type Asset struct {
	Symbol                 string      // e.g. GOOG, BRK.B, BTC/USD
	Class                  AssetClass  // e.g. AssetClassUSEquity, AssetClassCrypto
	Exchange               Exchange    // e.g. ExchangeNYSE, ExchangeNASDAQ, ExchangeCrypto
	Status                 AssetStatus // e.g. AssetStatusActive, AssetStatusInactive
	Name                   string      // e.g. Alphabet Inc. Class C Capital Stock, Bitcoin / US Dollar
	ID                     string      // e.g. 60d10d62-7876-415d-9feb-c92cd87076da
	Tradable               bool
	Marginable             bool
	Shortable              bool
	EasyToBorrow           bool
	Fractionable           bool
	MarginRequirementLong  decimal.Decimal // e.g. "0.3" for most equities
	MarginRequirementShort decimal.Decimal // e.g. "0.3" for most equities
	MinTradeIncrement      decimal.Decimal // e.g. 0.000000001 for crypto, otherwise 1
	PriceIncrement         decimal.Decimal // e.g. 0.000000001 for crypto, otherwise 0.01
}

// GetAssets retrieves all tradeable assets.
func (c *Client) GetAssets() (map[string]*Asset, error) {

	// generate cache key of today's date
	today := time.Now()
	key := today.Year()*10000 + int(today.Month())*100 + today.Day()

	// check cache for result with cheap read lock
	if res := func(key int) map[string]*Asset {
		assetsCacheMu.RLock()
		defer assetsCacheMu.RUnlock()
		if assetsCacheKey == key && assetsCacheData != nil {
			return assetsCacheData
		}
		return nil
	}(key); res != nil {
		return res, nil
	}

	// ensure only one fetch
	assetsCacheMu.Lock()
	defer assetsCacheMu.Unlock()

	// return cache if another goroutine updated it
	if assetsCacheKey == key && assetsCacheData != nil {
		return assetsCacheData, nil
	}

	// load or fetch the assets
	var err error
	var jsonAssets []jsonAsset
	if ds.IsOffline() {
		jsonAssets, err = c.loadAssets()
		if err != nil {
			panic(err)
		}
	} else {
		jsonAssets, err = c.fetchAssets()
		if err != nil {
			return nil, err
		}
	}

	// make the data structure more pleasant
	result, err := translateAssets(jsonAssets)
	if err != nil {
		return nil, err
	}

	// update cache
	assetsCacheKey = key
	assetsCacheData = result
	return result, nil
}

//go:embed assets.json
var assetsJSON []byte

// Cache for GetAssets - only refetches once per day in live mode
var (
	assetsCacheMu   sync.RWMutex
	assetsCacheKey  int // YYYYMMDD format
	assetsCacheData map[string]*Asset
)

type jsonAsset struct {
	ID                     string `json:"id"`       // e.g. 60d10d62-7876-415d-9feb-c92cd87076da
	Class                  string `json:"class"`    // e.g. us_equity, crypto
	Exchange               string `json:"exchange"` // e.g. NYSE, CRYPTO
	Symbol                 string `json:"symbol"`   // e.g. BTC/USD
	Name                   string `json:"name"`     // e.g. Bitcoin / US Dollar
	Status                 string `json:"status"`   // e.g. active
	Tradable               bool   `json:"tradable"`
	Marginable             bool   `json:"marginable"`
	Shortable              bool   `json:"shortable"`
	EasyToBorrow           bool   `json:"easy_to_borrow"`
	Fractionable           bool   `json:"fractionable"`
	MinTradeIncrement      string `json:"min_trade_increment,omitempty"`
	PriceIncrement         string `json:"price_increment,omitempty"`
	MarginRequirementLong  string `json:"margin_requirement_long"`  // e.g. "100"
	MarginRequirementShort string `json:"margin_requirement_short"` // e.g. "30"
}

func (c *Client) loadAssets() ([]jsonAsset, error) {
	var result []jsonAsset
	if err := json.Unmarshal(assetsJSON, &result); err != nil {
		return nil, fmt.Errorf("decoding embedded assets: %w", err)
	}
	return result, nil
}

func (c *Client) fetchAssets() ([]jsonAsset, error) {
	resp, err := c.Get("/v2/assets")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result []jsonAsset
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return result, nil
}

func translateAssets(jsonAssets []jsonAsset) (map[string]*Asset, error) {
	result := make(map[string]*Asset)
	for _, ja := range jsonAssets {
		ac, err := ParseAssetClass(ja.Class)
		if err != nil {
			log.Printf("[alpaca] unknown asset class for %s: %s", ja.Symbol, ja.Class)
			continue
		}
		ex, err := ParseExchange(ja.Exchange)
		if err != nil {
			log.Printf("[alpaca] unknown exchange for %s: %s", ja.Symbol, ja.Exchange)
			continue
		}
		as, err := ParseAssetStatus(ja.Status)
		if err != nil {
			log.Printf("[alpaca] unknown asset status for %s: %s", ja.Symbol, ja.Status)
			continue
		}
		minTradeIncrement := decimal.One
		if ja.MinTradeIncrement != "" {
			minTradeIncrement = decimal.Parse(ja.MinTradeIncrement)
		}
		priceIncrement := decimal.Cent
		if ja.PriceIncrement != "" {
			priceIncrement = decimal.Parse(ja.PriceIncrement)
		}
		result[ja.Symbol] = &Asset{
			Symbol:                 ja.Symbol,
			Class:                  ac,
			Exchange:               ex,
			Status:                 as,
			ID:                     ja.ID,
			Name:                   ja.Name,
			Tradable:               ja.Tradable,
			Marginable:             ja.Marginable,
			Shortable:              ja.Shortable,
			EasyToBorrow:           ja.EasyToBorrow,
			Fractionable:           ja.Fractionable,
			MarginRequirementLong:  decimal.Parse(ja.MarginRequirementLong).DivInt(100),
			MarginRequirementShort: decimal.Parse(ja.MarginRequirementShort).DivInt(100),
			MinTradeIncrement:      minTradeIncrement,
			PriceIncrement:         priceIncrement,
		}
	}
	return result, nil
}
