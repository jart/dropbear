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
	lock                  sync.Mutex
	sharesTradedThisMonth int64
	currentMonth          int // 1-12
}

// Fee constants (per share unless noted)
var (
	catFeePerTrade           = decimal.Parse("0.0003")  // CAT fee per trade
	tafFeePerShare           = decimal.Parse("0.0002")  // TAF fee per share
	exchangeTakerFeePerShare = decimal.Parse("0.0020")  // market orders
	exchangeMakerFeePerShare = decimal.Parse("-0.0018") // limit orders (rebate!)
	costPlusTier1Fee         = decimal.Parse("0.0025")  // <200k shares
	costPlusTier2Fee         = decimal.Parse("0.0020")  // 200k-1M shares
	costPlusTier3Fee         = decimal.Parse("0.0015")  // 1M-10M shares
	costPlusTier4Fee         = decimal.Parse("0.0010")  // 10M-50M shares
	costPlusTier5Fee         = decimal.Parse("0.0005")  // >50M shares
)

// NewAlpacaEliteFees creates a new fee calculator.
func NewAlpacaEliteFees() *AlpacaEliteFees {
	return &AlpacaEliteFees{}
}

// Calculate computes the fee for an order.
// Returns negative values for maker rebates on limit orders.
func (f *AlpacaEliteFees) Calculate(now clocky.Time, quantity int, isMarketOrder bool) decimal.Decimal {
	if quantity <= 0 {
		panic("quantity must be positive")
	}

	// track monthly volume
	sharesTradedThisMonth := func() int64 {
		f.lock.Lock()
		defer f.lock.Unlock()
		month := now.Month()
		if f.currentMonth != month {
			f.sharesTradedThisMonth = 0
			f.currentMonth = month
		}
		f.sharesTradedThisMonth += int64(quantity)
		return f.sharesTradedThisMonth
	}()

	// broker fee (volume-tiered)
	qty := decimal.FromInt(quantity)
	brokerFeePerShare := getBrokerFeePerShare(sharesTradedThisMonth)
	brokerFee := qty.Mul(brokerFeePerShare)

	// exchange fee (taker for market, maker for limit)
	var exchangeFee decimal.Decimal
	if isMarketOrder {
		exchangeFee = qty.Mul(exchangeTakerFeePerShare)
	} else {
		exchangeFee = qty.Mul(exchangeMakerFeePerShare)
	}

	// regulatory fees
	catFee := catFeePerTrade
	tafFee := qty.Mul(tafFeePerShare)
	regulatoryFees := catFee.Add(tafFee)

	// total fee (can be negative for large limit orders due to maker rebate)
	return brokerFee.Add(exchangeFee).Add(regulatoryFees)
}

// getBrokerFeePerShare returns the volume-tiered broker fee per share.
func getBrokerFeePerShare(sharesTradedThisMonth int64) decimal.Decimal {
	vol := sharesTradedThisMonth
	switch {
	case vol < 200_000:
		return costPlusTier1Fee
	case vol < 1_000_000:
		return costPlusTier2Fee
	case vol < 10_000_000:
		return costPlusTier3Fee
	case vol < 50_000_000:
		return costPlusTier4Fee
	default:
		return costPlusTier5Fee
	}
}
