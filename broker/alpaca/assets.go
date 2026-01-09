package alpaca

import (
	"fmt"
	"sync"

	"dropbear/decimal"
	"dropbear/ds/symbol"
	"dropbear/netty"
)

var Lock sync.RWMutex

// GetAsset retrieves an asset by symbol.
func GetAsset(sym symbol.Symbol) *Asset {
	Lock.RLock()
	defer Lock.RUnlock()
	return Assets[sym]
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
	shouldDelete := make(map[symbol.Symbol]bool)
	for _, asset := range Assets {
		shouldDelete[asset.Symbol] = true
	}
	for _, ja := range jsonAssets {
		sym, err := symbol.Parse(ja.Symbol)
		if err != nil {
			continue // skip symbols that are too long
		}
		shouldDelete[sym] = false
		minTradeIncrement := decimal.One
		if ja.MinTradeIncrement != "" {
			minTradeIncrement = decimal.Parse(ja.MinTradeIncrement)
		}
		priceIncrement := decimal.Cent
		if ja.PriceIncrement != "" {
			priceIncrement = decimal.Parse(ja.PriceIncrement)
		}
		asset := Assets[sym]
		name := ja.Name
		if name == "" {
			name = ja.Symbol
		}
		if asset == nil {
			asset = &Asset{
				Symbol:   sym,
				Exchange: ja.Exchange,
				Class:    ja.Class,
				Status:   ja.Status,
				ID:       ja.ID,
				Name:     name,
			}
			Assets[sym] = asset
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
		asset.Name = name
		asset.Status = ja.Status
		asset.Exchange = ja.Exchange
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
		asset.MarginRequirementLong.Store(ja.MarginRequirementLong.DivInt(100))
		asset.MarginRequirementShort.Store(ja.MarginRequirementShort.DivInt(100))
		asset.MinTradeIncrement.Store(minTradeIncrement)
		asset.PriceIncrement.Store(priceIncrement)
		asset.Status.Store(ja.Status)
	}
	for sym, del := range shouldDelete {
		if del {
			delete(Assets, sym)
		}
	}
	return nil
}

type jsonAsset struct {
	ID                     string          `json:"id"`       // e.g. 60d10d62-7876-415d-9feb-c92cd87076da
	Class                  AssetClass      `json:"class"`    // e.g. us_equity, crypto
	Exchange               Exchange        `json:"exchange"` // e.g. NYSE, CRYPTO
	Symbol                 string          `json:"symbol"`   // e.g. BTC/USD
	Name                   string          `json:"name"`     // e.g. Bitcoin / US Dollar
	Status                 AssetStatus     `json:"status"`   // e.g. active
	Tradable               bool            `json:"tradable"`
	Marginable             bool            `json:"marginable"`
	Shortable              bool            `json:"shortable"`
	EasyToBorrow           bool            `json:"easy_to_borrow"`
	Fractionable           bool            `json:"fractionable"`
	MinTradeIncrement      string          `json:"min_trade_increment,omitempty"`
	PriceIncrement         string          `json:"price_increment,omitempty"`
	MarginRequirementLong  decimal.Decimal `json:"margin_requirement_long"`  // e.g. "100"
	MarginRequirementShort decimal.Decimal `json:"margin_requirement_short"` // e.g. "30"
	Attributes             []string        `json:"attributes,omitempty"`
}

func (c *Client) fetchAssets() ([]jsonAsset, error) {
	var jsonAssets []jsonAsset
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.BulkHttpClient, "GET", "/v2/assets", nil, &jsonAssets)
	if err != nil {
		return nil, err
	}
	return jsonAssets, nil
}
