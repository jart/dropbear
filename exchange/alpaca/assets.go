package alpaca

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"dropbear/ds"
)

//go:embed assets.json
var assetsJSON []byte

// Asset represents something you can trade.
type Asset struct {
	ID                     string `json:"id"`       // e.g. 60d10d62-7876-415d-9feb-c92cd87076da
	Class                  string `json:"class"`    // e.g. us_equity, crypto
	Exchange               string `json:"exchange"` // e.g. CRYPTO
	Symbol                 string `json:"symbol"`   // e.g. BTC/USD
	Name                   string `json:"name"`     // e.g. Bitcoin / US Dollar
	Status                 string `json:"status"`   // e.g. active
	Tradable               bool   `json:"tradable"`
	Marginable             bool   `json:"marginable"`
	Shortable              bool   `json:"shortable"`
	EasyToBorrow           bool   `json:"easy_to_borrow"`
	Fractionable           bool   `json:"fractionable"`
	MinTradeIncrement      string `json:"min_trade_increment"`
	PriceIncrement         string `json:"price_increment"`
	MarginRequirementLong  string `json:"margin_requirement_long"`  // e.g. "100"
	MarginRequirementShort string `json:"margin_requirement_short"` // e.g. "30"
}

// GetAssets retrieves all tradeable assets.
func (c *Client) GetAssets() ([]Asset, error) {
	if ds.IsOffline() {
		var result []Asset
		if err := json.Unmarshal(assetsJSON, &result); err != nil {
			return nil, fmt.Errorf("decoding embedded assets: %w", err)
		}
		return result, nil
	}
	resp, err := c.Get("/v2/assets")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result []Asset
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return result, nil
}
