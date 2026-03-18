package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"sync"
)

var (
	catFeePerTrade           = decimal.Parse("0.0003")  // CAT fee per trade
	tafFeePerShare           = decimal.Parse("0.0002")  // TAF fee per share
	exchangeTakerFeePerShare = decimal.Parse("0.0020")  // market orders
	exchangeMakerFeePerShare = decimal.Parse("-0.0018") // limit orders (rebate!)
)

var costPlusFeeTiers = []struct {
	threshold decimal.Decimal
	fee       decimal.Decimal
}{
	{decimal.FromInt(50_000_000), decimal.Parse("0.0005")},
	{decimal.FromInt(10_000_000), decimal.Parse("0.0010")},
	{decimal.FromInt(1_000_000), decimal.Parse("0.0015")},
	{decimal.FromInt(200_000), decimal.Parse("0.0020")},
	{decimal.Zero, decimal.Parse("0.0025")},
}

// FeeCalculator implements the Alpaca Elite cost-plus fee structure.
type FeeCalculator struct {
	TotalFees    decimal.Decimal
	lock         sync.Mutex
	sharesTraded decimal.Decimal
	currentMonth clocky.Month
}

// NewFeeCalculator creates a new fee calculator.
func NewFeeCalculator() *FeeCalculator {
	return &FeeCalculator{}
}

// GetFee calculates the fee for a trade.
// Might return a negative fee for maker orders.
func (f *FeeCalculator) GetFee(now clocky.Time, sharesTraded decimal.Decimal, isMarketable bool) decimal.Decimal {
	quantity := sharesTraded.Abs()
	sharesTradedThisMonth := f.getSharesTradedThisMonth(now, quantity)
	brokerFeePerShare := getBrokerFeePerShare(sharesTradedThisMonth)
	brokerFee := quantity.Mul(brokerFeePerShare)
	var exchangeFee decimal.Decimal
	if isMarketable {
		exchangeFee = quantity.Mul(exchangeTakerFeePerShare)
	} else {
		exchangeFee = quantity.Mul(exchangeMakerFeePerShare)
	}
	catFee := catFeePerTrade
	tafFee := quantity.Mul(tafFeePerShare)
	regulatoryFees := catFee.Add(tafFee)
	fee := brokerFee.Add(exchangeFee).Add(regulatoryFees)
	f.TotalFees.AtomicAdd(fee)
	return fee
}

func (f *FeeCalculator) getSharesTradedThisMonth(now clocky.Time, quantity decimal.Decimal) decimal.Decimal {
	month := now.Month()
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.currentMonth != month {
		f.sharesTraded = decimal.Zero
		f.currentMonth = month
	}
	f.sharesTraded = f.sharesTraded.Add(quantity)
	return f.sharesTraded
}

func getBrokerFeePerShare(sharesTradedThisMonth decimal.Decimal) decimal.Decimal {
	for _, tier := range costPlusFeeTiers {
		if sharesTradedThisMonth.Cmp(tier.threshold) >= 0 {
			return tier.fee
		}
	}
	panic("impossible")
}
