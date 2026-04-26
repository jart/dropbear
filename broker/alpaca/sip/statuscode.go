package sip

import "fmt"

// StatusCode represents a trading status code from the SIP feed.
// CTA (Tape A/B) and UTP (Tape C) use different code sets.
type StatusCode byte

// CTA Status Codes (Tape A & B)
const (
	StatusCodeTradingHaltCTA         StatusCode = '2' // Trading Halt
	StatusCodeResumeCTA              StatusCode = '3' // Resume
	StatusCodePriceIndication        StatusCode = '5' // Price Indication
	StatusCodeTradingRangeIndication StatusCode = '6' // Trading Range Indication
	StatusCodeMarketImbalanceBuy     StatusCode = '7' // Market Imbalance Buy: A 50,000 share or more excess of market orders to buy over market orders to sell as of 9:00 a.m. on expiration days
	StatusCodeMarketImbalanceSell    StatusCode = '8' // Market Imbalance Sell: A 50,000 share or more, excess of market orders to sell over market orders to buy as of 9:00 a.m. on expiration days.
	StatusCodeMOCImbalanceBuy        StatusCode = '9' // Market On Close Imbalance Buy: Excess closing auction interest to buy, above a pre-defined threshold as determined by the primary listing market.
	StatusCodeMOCImbalanceSell       StatusCode = 'A' // Market On Close Imbalance Sell: Excess closing auction interest to sell, above a pre-defined threshold as determined by the primary listing market.
	StatusCodeNoMarketImbalance      StatusCode = 'C' // No Market Imbalance
	StatusCodeNoMOCImbalance         StatusCode = 'D' // No Market On Close Imbalance
	StatusCodeShortSaleRestriction   StatusCode = 'E' // Short Sale Restriction
	StatusCodeLimitUpLimitDown       StatusCode = 'F' // Limit Up-Limit Down
)

// UTP Status Codes (Tape C & O)
const (
	StatusCodeTradingHaltUTP       StatusCode = 'H' // Trading Halt
	StatusCodeVolatilityPause      StatusCode = 'P' // Volatility Trading Pause
	StatusCodeQuotationResumption  StatusCode = 'Q' // Quotation Resumption
	StatusCodeTradingResumptionUTP StatusCode = 'T' // Trading Resumption
)

var statusCodeJSON [256][]byte

func init() {
	for i := range statusCodeJSON {
		statusCodeJSON[i] = []byte{'"', byte(i), '"'}
	}
}

func (c StatusCode) GoString() string {
	switch c {
	case StatusCodeTradingHaltCTA:
		return "sip.StatusCodeTradingHaltCTA"
	case StatusCodeResumeCTA:
		return "sip.StatusCodeResumeCTA"
	case StatusCodePriceIndication:
		return "sip.StatusCodePriceIndication"
	case StatusCodeTradingRangeIndication:
		return "sip.StatusCodeTradingRangeIndication"
	case StatusCodeMarketImbalanceBuy:
		return "sip.StatusCodeMarketImbalanceBuy"
	case StatusCodeMarketImbalanceSell:
		return "sip.StatusCodeMarketImbalanceSell"
	case StatusCodeMOCImbalanceBuy:
		return "sip.StatusCodeMOCImbalanceBuy"
	case StatusCodeMOCImbalanceSell:
		return "sip.StatusCodeMOCImbalanceSell"
	case StatusCodeNoMarketImbalance:
		return "sip.StatusCodeNoMarketImbalance"
	case StatusCodeNoMOCImbalance:
		return "sip.StatusCodeNoMOCImbalance"
	case StatusCodeShortSaleRestriction:
		return "sip.StatusCodeShortSaleRestriction"
	case StatusCodeLimitUpLimitDown:
		return "sip.StatusCodeLimitUpLimitDown"
	case StatusCodeTradingHaltUTP:
		return "sip.StatusCodeTradingHaltUTP"
	case StatusCodeVolatilityPause:
		return "sip.StatusCodeVolatilityPause"
	case StatusCodeQuotationResumption:
		return "sip.StatusCodeQuotationResumption"
	case StatusCodeTradingResumptionUTP:
		return "sip.StatusCodeTradingResumptionUTP"
	default:
		return fmt.Sprintf("sip.StatusCode('%c')", c)
	}
}

func (c StatusCode) String() string {
	return c.GoString()
}

func (c StatusCode) MarshalJSON() ([]byte, error) {
	return statusCodeJSON[c], nil
}

func (c *StatusCode) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		*c = StatusCode(data[1])
		return nil
	}
	return ErrInvalidStatusCode
}
