package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"sync"
)

// InterestCalculator tracks margin interest charges on negative cash balances.
type InterestCalculator struct {
	TotalCharged   decimal.Decimal
	lock           sync.Mutex
	spreadOverRFR  decimal.Decimal
	yearlyCharged  map[int]decimal.Decimal
	lastChargeDate int
}

// NewInterestCalculator creates a margin interest tracker.
// spreadOverRFR is how much more over risk-free rate Alpaca takes, e.g. 1% = 0.01.
func NewInterestCalculator(spreadOverRFR decimal.Decimal) *InterestCalculator {
	return &InterestCalculator{
		spreadOverRFR: spreadOverRFR,
		yearlyCharged: make(map[int]decimal.Decimal),
	}
}

// GetDailyInterest should be called at end of each trading day.
// Returns the amount charged (positive if interest was charged).
func (m *InterestCalculator) GetDailyInterest(now clocky.Time, cashBalance, riskFreeRate decimal.Decimal) decimal.Decimal {
	m.lock.Lock()
	defer m.lock.Unlock()

	// only charge if cash balance is negative (using margin)
	if !cashBalance.IsNegative() {
		return decimal.Zero
	}

	// avoid charging multiple times on same day
	today := now.DateInt()
	if m.lastChargeDate != 0 && m.lastChargeDate == today {
		return decimal.Zero
	}
	m.lastChargeDate = today

	// calculate daily interest
	// annual rate = risk-free rate + spread
	// daily rate = annual rate / 360 (industry standard)
	annualRate := riskFreeRate.Add(m.spreadOverRFR)
	dailyRate := annualRate.DivInt(360)

	// debit balance is the absolute value of negative cash
	debitBalance := cashBalance.Neg()
	dailyCharge := debitBalance.Mul(dailyRate)

	// charge 3 days on Fridays (weekend)
	daysToCharge := 1
	if now.Weekday() == clocky.Friday {
		daysToCharge = 3
	}
	totalCharge := dailyCharge.MulInt(daysToCharge)

	// track totals
	m.TotalCharged.AtomicAdd(totalCharge)
	year := now.Year()
	if _, ok := m.yearlyCharged[year]; !ok {
		m.yearlyCharged[year] = decimal.Zero
	}
	m.yearlyCharged[year] = m.yearlyCharged[year].Add(totalCharge)

	return totalCharge
}
