package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"sync"
)

// AlpacaEliteFees implements the Alpaca Elite cost-plus fee structure.
// This is more realistic than zero-fee simulation and essential for
// backtesting high-frequency strategies.
type AlpacaEliteFees struct {
	lock          sync.Mutex
	monthlyVolume int64 // shares traded this month
	currentMonth  int   // 1-12
}

// Fee constants (per share unless noted)
const (
	catFeePerTrade           = 0.0003 // CAT fee per trade
	tafFeePerShare           = 0.0002 // TAF fee per share
	exchangeTakerFeePerShare = 0.0020 // market orders
	exchangeMakerFeePerShare = -0.0018 // limit orders (rebate!)
)

// NewAlpacaEliteFees creates a new fee calculator.
func NewAlpacaEliteFees() *AlpacaEliteFees {
	return &AlpacaEliteFees{}
}

// Calculate computes the fee for an order.
// Returns negative values for maker rebates on limit orders.
func (f *AlpacaEliteFees) Calculate(now clocky.Time, quantity int, isMarketOrder bool) decimal.Decimal {
	if quantity <= 0 {
		return decimal.Zero
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	// Reset monthly volume on new month
	month := now.Month()
	if f.currentMonth != month {
		f.monthlyVolume = 0
		f.currentMonth = month
	}

	// Update monthly volume
	f.monthlyVolume += int64(quantity)

	qty := decimal.FromInt(quantity)

	// Broker fee (volume-tiered)
	brokerFee := qty.Mul(f.getBrokerFee())

	// Exchange fee (taker for market, maker for limit)
	var exchangeFee decimal.Decimal
	if isMarketOrder {
		exchangeFee = qty.Mul(decimal.FromFloat64(exchangeTakerFeePerShare))
	} else {
		exchangeFee = qty.Mul(decimal.FromFloat64(exchangeMakerFeePerShare))
	}

	// Regulatory fees
	catFee := decimal.FromFloat64(catFeePerTrade)
	tafFee := qty.Mul(decimal.FromFloat64(tafFeePerShare))
	regulatoryFees := catFee.Add(tafFee)

	// Total fee (can be negative for large limit orders due to maker rebate)
	totalFee := brokerFee.Add(exchangeFee).Add(regulatoryFees)

	return totalFee
}

// getBrokerFee returns the volume-tiered broker fee per share.
func (f *AlpacaEliteFees) getBrokerFee() decimal.Decimal {
	vol := f.monthlyVolume
	switch {
	case vol < 200_000:
		return decimal.FromFloat64(0.0025)
	case vol < 1_000_000:
		return decimal.FromFloat64(0.0020)
	case vol < 10_000_000:
		return decimal.FromFloat64(0.0015)
	case vol < 50_000_000:
		return decimal.FromFloat64(0.0010)
	default:
		return decimal.FromFloat64(0.0005)
	}
}

// GetMonthlyVolume returns the current monthly trading volume.
func (f *AlpacaEliteFees) GetMonthlyVolume() int64 {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.monthlyVolume
}
