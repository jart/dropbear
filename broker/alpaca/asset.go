package alpaca

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/symbol"
)

var (
	lowPriceLongThreshold  = decimal.Parse("2.50") // 100% margin for longs < $2.50
	lowPriceShortThreshold = decimal.Parse("5.00") // special rules for shorts < $5.00
	shortMinPerShareLow    = decimal.Parse("2.50") // min $2.50/share for stocks < $5
	shortMinPerShareHigh   = decimal.Parse("5.00") // min $5.00/share for stocks >= $5
)

type Asset struct {
	Symbol                 symbol.Symbol   // e.g. GOOG, BRK.B, BTC/USD
	Name                   string          // e.g. Alphabet Inc. Class C Capital Stock, Bitcoin / US Dollar
	ID                     string          // e.g. 60d10d62-7876-415d-9feb-c92cd87076da
	Exchange               Exchange        // e.g. ExchangeNYSE, ExchangeNASDAQ, ExchangeCrypto
	Class                  AssetClass      // e.g. AssetClassUSEquity, AssetClassCrypto
	Status                 AssetStatus     // e.g. AssetStatusActive, AssetStatusInactive
	IPO                    ds.Bool         // currently in initial public offering phase (limit orders only)
	Tradable               ds.Bool         // can be bought or sold on Alpaca
	Marginable             ds.Bool         // can be purchased with borrowed funds (margin)
	Shortable              ds.Bool         // can be sold short (subject to availability)
	EasyToBorrow           ds.Bool         // readily available for shorting (no locate required)
	Fractionable           ds.Bool         // supports fractional shares during regular market hours
	FractionableExtHours   ds.Bool         // supports fractional shares during extended hours
	HasOptions             ds.Bool         // options contracts are available for this asset
	OptionsLateClose       ds.Bool         // options trade 15 min past close
	PTPNoException         ds.Bool         // publicly traded partnership subject to 10% withholding
	PTPWithException       ds.Bool         // publicly traded partnership currently exempt from withholding
	OvernightTradable      ds.Bool         // indicates asset is eligable for overnight session
	OvernightHalted        ds.Bool         // indicates asset is currently halted in the overnight session
	MarginRequirementLong  decimal.Decimal // initial margin requirement to buy this asset (e.g., 0.25 for 25%)
	MarginRequirementShort decimal.Decimal // maintenance margin requirement to hold a short position (e.g., 0.30 for 30%)
	MinTradeIncrement      decimal.Decimal // smallest amount of the asset that can be traded (e.g., 0.000000001 for crypto)
	PriceIncrement         decimal.Decimal // smallest unit of price movement supported (tick size)
}

var stardardMarginRate = decimal.Parse("0.3")

func (a *Asset) IsHealthy() bool {
	return a.Class == AssetClassUSEquity &&
		a.Status == AssetStatusActive &&
		a.Tradable.Load() &&
		a.Marginable.Load() &&
		a.HasOptions.Load() &&
		a.EasyToBorrow.Load() &&
		!a.PTPNoException.Load() &&
		!a.PTPWithException.Load() &&
		a.MarginRequirementLong.Load().Cmp(stardardMarginRate) == 0 &&
		a.MarginRequirementShort.Load().Cmp(stardardMarginRate) == 0
}

func (asset *Asset) String() string {
	return asset.Symbol.String()
}

// GetInitialMargin calculates the initial margin required to open a position.
func (asset *Asset) GetInitialMargin(quantity, price decimal.Decimal) decimal.Decimal {
	value := quantity.Abs().Mul(price)
	maintenanceMargin := asset.GetMaintenanceMargin(quantity, price)
	return maintenanceMargin.Max(value.Mul(decimal.Half))
}

// GetMaintenanceMargin calculates the maintenance margin required for a position.
func (asset *Asset) GetMaintenanceMargin(quantity, price decimal.Decimal) decimal.Decimal {
	absQty := quantity.Abs()
	value := absQty.Mul(price)
	if quantity.IsNegative() {
		marginByRate := value.Mul(asset.MarginRequirementShort)
		if price.Cmp(lowPriceShortThreshold) < 0 {
			// for stocks < $5: 100% or $2.50/share, whichever is greater
			minPerShare := absQty.Mul(shortMinPerShareLow)
			return marginByRate.Max(minPerShare).Max(value)
		} else {
			// for stocks >= $5: rate or $5.00/share, whichever is greater
			minPerShare := absQty.Mul(shortMinPerShareHigh)
			return marginByRate.Max(minPerShare)
		}
	} else {
		pct := asset.MarginRequirementLong
		if price.Cmp(lowPriceLongThreshold) < 0 {
			pct = pct.Max(decimal.One) // low-priced stocks require 100% margin
		}
		return pct.Mul(value)
	}
}
