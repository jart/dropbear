package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"log"
	"sync"
)

// MarginInterest tracks margin interest charges on negative cash balances.
// Alpaca charges interest on any debit balance at EOD.
type MarginInterest struct {
	lock           sync.Mutex
	spreadOverRFR  decimal.Decimal // additional spread over risk-free rate (e.g., 1% = 0.01)
	totalCharged   decimal.Decimal
	yearlyCharged  map[int]decimal.Decimal
	lastChargeDate clocky.Time
}

// NewMarginInterest creates a margin interest tracker.
// spreadBPS is the spread over the risk-free rate in basis points (e.g., 100 = 1%).
func NewMarginInterest(spreadBPS int) *MarginInterest {
	return &MarginInterest{
		spreadOverRFR: decimal.FromBPS(spreadBPS),
		yearlyCharged: make(map[int]decimal.Decimal),
	}
}

// ChargeDaily should be called at end of each trading day.
// It charges interest on negative cash balances.
// Returns the amount charged (positive if interest was charged).
func (m *MarginInterest) ChargeDaily(now clocky.Time, cashBalance decimal.Decimal) decimal.Decimal {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Only charge if cash balance is negative (using margin)
	if !cashBalance.IsNegative() {
		return decimal.Zero
	}

	// Avoid charging multiple times on same day
	today := now.Quantize(clocky.Day)
	if !m.lastChargeDate.IsZero() && m.lastChargeDate == today {
		return decimal.Zero
	}
	m.lastChargeDate = today

	// Calculate daily interest
	// Annual rate = risk-free rate + spread
	// Daily rate = annual rate / 360 (industry standard)
	riskFreeRate := GetRiskFreeRate()
	annualRate := riskFreeRate.Add(m.spreadOverRFR)
	dailyRate := annualRate.DivInt(360)

	// Debit balance is the absolute value of negative cash
	debitBalance := cashBalance.Neg()
	dailyCharge := debitBalance.Mul(dailyRate)

	// Charge 3 days on Fridays (weekend)
	daysToCharge := 1
	if now.Weekday() == clocky.Friday {
		daysToCharge = 3
	}
	totalCharge := dailyCharge.MulInt(daysToCharge)

	// Track totals
	m.totalCharged = m.totalCharged.Add(totalCharge)
	year := now.Year()
	if _, ok := m.yearlyCharged[year]; !ok {
		m.yearlyCharged[year] = decimal.Zero
	}
	m.yearlyCharged[year] = m.yearlyCharged[year].Add(totalCharge)

	if *flagVerbose {
		log.Printf("[margin] EOD interest: $%s (debit: $%s, rate: %s%%, YTD: $%s)",
			totalCharge, debitBalance, annualRate.MulInt(100), m.yearlyCharged[year])
	}

	return totalCharge
}

// GetTotalCharged returns total margin interest charged.
func (m *MarginInterest) GetTotalCharged() decimal.Decimal {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.totalCharged
}

// GetYearlyCharged returns margin interest charged for a specific year.
func (m *MarginInterest) GetYearlyCharged(year int) decimal.Decimal {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.yearlyCharged[year]
}

// MaintenanceMarginRate returns the maintenance margin rate for a stock.
// Default is 30% for most stocks, but varies for volatile/meme stocks.
func MaintenanceMarginRate(symbol string) decimal.Decimal {
	assets, _ := AlpacaClient.GetAssets()
	if asset, ok := assets[symbol]; ok {
		return asset.MarginRequirementLong
	}
	return decimal.Parse("0.30") // default 30%
}

// MaintenanceMarginRateShort returns the maintenance margin rate for shorting a stock.
func MaintenanceMarginRateShort(symbol string) decimal.Decimal {
	assets, _ := AlpacaClient.GetAssets()
	if asset, ok := assets[symbol]; ok {
		return asset.MarginRequirementShort
	}
	return decimal.Parse("0.30") // default 30%
}

// InitialMarginRate returns the initial margin requirement (Reg-T).
// This is max(50%, maintenance rate) for most stocks.
func InitialMarginRate(symbol string) decimal.Decimal {
	return MaintenanceMarginRate(symbol).Max(decimal.Half)
}

// InitialMarginRateShort returns the initial margin requirement for shorting.
func InitialMarginRateShort(symbol string) decimal.Decimal {
	return MaintenanceMarginRateShort(symbol).Max(decimal.Half)
}

// Thresholds for special margin rules
var (
	lowPriceLongThreshold  = decimal.Parse("2.50") // 100% margin for longs < $2.50
	lowPriceShortThreshold = decimal.Parse("5.00") // special rules for shorts < $5.00
	shortMinPerShareLow    = decimal.Parse("2.50") // min $2.50/share for stocks < $5
	shortMinPerShareHigh   = decimal.Parse("5.00") // min $5.00/share for stocks >= $5
)

// MaintenanceMargin calculates the maintenance margin required for a position.
// For long positions: rate * abs(quantity * price)
// For short positions: max(rate * value, min_per_share * quantity)
// Special rules apply for low-priced stocks.
func MaintenanceMargin(symbol string, quantity, price decimal.Decimal) decimal.Decimal {
	if quantity.IsZero() {
		return decimal.Zero
	}

	absQty := quantity.Abs()
	value := absQty.Mul(price)

	if quantity.IsPositive() {
		// Long position
		if price.Cmp(lowPriceLongThreshold) < 0 {
			// Low-priced stocks require 100% margin
			return value
		}
		rate := MaintenanceMarginRate(symbol).Max(decimal.Parse("0.30"))
		return value.Mul(rate)
	}

	// Short position
	rate := MaintenanceMarginRateShort(symbol).Max(decimal.Parse("0.30"))
	marginByRate := value.Mul(rate)

	if price.Cmp(lowPriceShortThreshold) < 0 {
		// For stocks < $5: 100% or $2.50/share, whichever is greater
		minPerShare := absQty.Mul(shortMinPerShareLow)
		return marginByRate.Max(minPerShare).Max(value)
	}

	// For stocks >= $5: rate or $5.00/share, whichever is greater
	minPerShare := absQty.Mul(shortMinPerShareHigh)
	return marginByRate.Max(minPerShare)
}

// InitialMargin calculates the initial margin required to open a position.
// Reg-T requires at least 50% for all marginable securities.
func InitialMargin(symbol string, quantity, price decimal.Decimal) decimal.Decimal {
	if quantity.IsZero() {
		return decimal.Zero
	}

	absQty := quantity.Abs()
	value := absQty.Mul(price)

	if quantity.IsPositive() {
		// Long position
		if price.Cmp(lowPriceLongThreshold) < 0 {
			// Low-priced stocks require 100% margin
			return value
		}
		// Reg-T: max(50%, maintenance rate)
		rate := InitialMarginRate(symbol)
		return value.Mul(rate)
	}

	// Short position
	rate := InitialMarginRateShort(symbol)
	marginByRate := value.Mul(rate)

	if price.Cmp(lowPriceShortThreshold) < 0 {
		// For stocks < $5: 100% or $2.50/share, whichever is greater
		minPerShare := absQty.Mul(shortMinPerShareLow)
		return marginByRate.Max(minPerShare).Max(value)
	}

	// For stocks >= $5: max(50%, rate) or $5.00/share, whichever is greater
	minPerShare := absQty.Mul(shortMinPerShareHigh)
	return marginByRate.Max(minPerShare)
}

// IsMarginable returns whether a symbol can be traded on margin.
func IsMarginable(symbol string) bool {
	assets, _ := AlpacaClient.GetAssets()
	if asset, ok := assets[symbol]; ok {
		return asset.Marginable
	}
	return false
}

// IsShortable returns whether a symbol can be sold short.
func IsShortable(symbol string) bool {
	assets, _ := AlpacaClient.GetAssets()
	if asset, ok := assets[symbol]; ok {
		return asset.Shortable
	}
	return false
}

// IsEasyToBorrow returns whether a symbol is easy to borrow for shorting.
func IsEasyToBorrow(symbol string) bool {
	assets, _ := AlpacaClient.GetAssets()
	if asset, ok := assets[symbol]; ok {
		return asset.EasyToBorrow
	}
	return false
}
