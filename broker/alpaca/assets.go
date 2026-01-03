package alpaca

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"dropbear/decimal"
)

var Lock sync.RWMutex

// GetAsset retrieves an asset by symbol.
func GetAsset(symbol string) *Asset {
	Lock.RLock()
	defer Lock.RUnlock()
	return Assets[symbol]
}

// SyncAssets fetches the latest assets from Alpaca and updates the local Assets map.
// Asset objects returned earlier by GetAsset are atomically updated in place.
func (c *Client) SyncAssets() error {
	Lock.Lock()
	defer Lock.Unlock()
	jsonAssets, err := c.fetchAssets()
	if err != nil {
		return err
	}
	shouldDelete := make(map[string]bool)
	for _, asset := range Assets {
		shouldDelete[asset.Symbol] = true
	}
	for _, ja := range jsonAssets {
		shouldDelete[ja.Symbol] = false
		ac, err := ParseAssetClass(ja.Class)
		if err != nil {
			err = fmt.Errorf("unknown asset class for %s: %s", ja.Symbol, ja.Class)
			continue
		}
		ex, err := ParseExchange(ja.Exchange)
		if err != nil {
			err = fmt.Errorf("unknown exchange for %s: %s", ja.Symbol, ja.Exchange)
			continue
		}
		as, err := ParseAssetStatus(ja.Status)
		if err != nil {
			err = fmt.Errorf("unknown asset status for %s: %s", ja.Symbol, ja.Status)
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
		asset := Assets[ja.Symbol]
		if asset == nil {
			name := ja.Name
			if name == "" {
				name = ja.Symbol
			}
			asset = &Asset{
				Symbol:   ja.Symbol,
				Exchange: ex,
				Class:    ac,
				Status:   as,
				ID:       ja.ID,
				Name:     name,
			}
			Assets[ja.Symbol] = asset
		}
		IPO := false
		HasOptions := false
		PTPNoException := false
		PTPWithException := false
		OptionsLateClose := false
		FractionableExtHours := false
		for _, attr := range ja.Attributes {
			switch attr {
			case "ipo":
				IPO = true
			case "has_options":
				HasOptions = true
			case "ptp_no_exception":
				PTPNoException = true
			case "ptp_with_exception":
				PTPWithException = true
			case "options_late_close":
				OptionsLateClose = true
			case "fractional_eh_enabled":
				FractionableExtHours = true
			default:
				err = fmt.Errorf("unknown asset attribute for %s: %s", ja.Symbol, attr)
			}
		}
		asset.IPO.Store(IPO)
		asset.HasOptions.Store(HasOptions)
		asset.PTPNoException.Store(PTPNoException)
		asset.PTPWithException.Store(PTPWithException)
		asset.OptionsLateClose.Store(OptionsLateClose)
		asset.FractionableExtHours.Store(FractionableExtHours)
		asset.Tradable.Store(ja.Tradable)
		asset.Marginable.Store(ja.Marginable)
		asset.Shortable.Store(ja.Shortable)
		asset.EasyToBorrow.Store(ja.EasyToBorrow)
		asset.Fractionable.Store(ja.Fractionable)
		asset.MarginRequirementLong.Store(decimal.Parse(ja.MarginRequirementLong).DivInt(100))
		asset.MarginRequirementShort.Store(decimal.Parse(ja.MarginRequirementShort).DivInt(100))
		asset.MinTradeIncrement.Store(minTradeIncrement)
		asset.PriceIncrement.Store(priceIncrement)
		asset.Status.Store(as)
	}
	for sym, del := range shouldDelete {
		if del {
			delete(Assets, sym)
		}
	}
	return err
}

type jsonAsset struct {
	ID                     string   `json:"id"`       // e.g. 60d10d62-7876-415d-9feb-c92cd87076da
	Class                  string   `json:"class"`    // e.g. us_equity, crypto
	Exchange               string   `json:"exchange"` // e.g. NYSE, CRYPTO
	Symbol                 string   `json:"symbol"`   // e.g. BTC/USD
	Name                   string   `json:"name"`     // e.g. Bitcoin / US Dollar
	Status                 string   `json:"status"`   // e.g. active
	Tradable               bool     `json:"tradable"`
	Marginable             bool     `json:"marginable"`
	Shortable              bool     `json:"shortable"`
	EasyToBorrow           bool     `json:"easy_to_borrow"`
	Fractionable           bool     `json:"fractionable"`
	MinTradeIncrement      string   `json:"min_trade_increment,omitempty"`
	PriceIncrement         string   `json:"price_increment,omitempty"`
	MarginRequirementLong  string   `json:"margin_requirement_long"`  // e.g. "100"
	MarginRequirementShort string   `json:"margin_requirement_short"` // e.g. "30"
	Attributes             []string `json:"attributes,omitempty"`
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
