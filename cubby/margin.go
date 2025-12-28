package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"log"
	"sync"
	"time"
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
		spreadOverRFR: decimal.FromInt(spreadBPS).DivInt(10000),
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
	if now.Weekday() == time.Friday {
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

// MaintenanceMarginRate returns the maintenance margin requirement for a stock.
// Default is 30% for most stocks, but varies for volatile/meme stocks.
func MaintenanceMarginRate(symbol string) decimal.Decimal {
	// Check for known exceptions
	if rate, ok := marginExceptions[symbol]; ok {
		return rate
	}
	// Default is 30% for equities
	return decimal.FromInt(30).DivInt(100)
}

// InitialMarginRate returns the initial margin requirement (Reg-T).
// This is 50% for most stocks.
func InitialMarginRate(symbol string) decimal.Decimal {
	maintenance := MaintenanceMarginRate(symbol)
	// Initial margin is always at least 50% (Reg-T)
	minInitial := decimal.FromInt(50).DivInt(100)
	return maintenance.Max(minInitial)
}

// marginExceptions contains stocks with non-standard maintenance margin requirements.
// Values are decimal percentages (e.g., 0.40 = 40% margin required).
var marginExceptions = map[string]decimal.Decimal{
	// High volatility stocks require higher margin
	"TSLA": decimal.FromInt(40).DivInt(100),
	"AMC":  decimal.FromInt(100).DivInt(100), // not marginable
	"GME":  decimal.FromInt(100).DivInt(100), // not marginable
	"PLTR": decimal.FromInt(100).DivInt(100), // not marginable
	"SNOW": decimal.FromInt(100).DivInt(100), // not marginable
	"BB":   decimal.FromInt(100).DivInt(100), // not marginable
	"CLOV": decimal.FromInt(100).DivInt(100), // not marginable
	"MSTR": decimal.FromInt(100).DivInt(100), // not marginable (crypto proxy)

	// Leveraged ETFs
	"SQQQ": decimal.FromInt(100).DivInt(100),
	"TQQQ": decimal.FromInt(100).DivInt(100),
	"UVXY": decimal.FromInt(100).DivInt(100),
	"VIXY": decimal.FromInt(100).DivInt(100),
	"TBT":  decimal.FromInt(100).DivInt(100),
}
