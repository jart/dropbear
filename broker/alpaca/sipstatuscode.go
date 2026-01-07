package alpaca

import "fmt"

// SIPStatusCode represents a trading status code from the SIP feed.
// CTA (Tape A/B) and UTP (Tape C) use different code sets.
type SIPStatusCode byte

// CTA Status Codes (Tape A & B)
const (
	StatusCodeTradingHaltCTA         SIPStatusCode = '2' // Trading Halt
	StatusCodeResumeCTA              SIPStatusCode = '3' // Resume
	StatusCodePriceIndication        SIPStatusCode = '5' // Price Indication
	StatusCodeTradingRangeIndication SIPStatusCode = '6' // Trading Range Indication
	StatusCodeMarketImbalanceBuy     SIPStatusCode = '7' // Market Imbalance Buy
	StatusCodeMarketImbalanceSell    SIPStatusCode = '8' // Market Imbalance Sell
	StatusCodeMOCImbalanceBuy        SIPStatusCode = '9' // Market On Close Imbalance Buy
	StatusCodeMOCImbalanceSell       SIPStatusCode = 'A' // Market On Close Imbalance Sell
	StatusCodeNoMarketImbalance      SIPStatusCode = 'C' // No Market Imbalance
	StatusCodeNoMOCImbalance         SIPStatusCode = 'D' // No Market On Close Imbalance
	StatusCodeShortSaleRestriction   SIPStatusCode = 'E' // Short Sale Restriction
	StatusCodeLimitUpLimitDown       SIPStatusCode = 'F' // Limit Up-Limit Down
)

// UTP Status Codes (Tape C)
const (
	StatusCodeTradingHaltUTP       SIPStatusCode = 'H' // Trading Halt
	StatusCodeVolatilityPause      SIPStatusCode = 'P' // Volatility Trading Pause
	StatusCodeQuotationResumption  SIPStatusCode = 'Q' // Quotation Resumption
	StatusCodeTradingResumptionUTP SIPStatusCode = 'T' // Trading Resumption
)

// Pre-computed JSON representations
var sipStatusCodeJSON [256][]byte

func init() {
	for i := range sipStatusCodeJSON {
		sipStatusCodeJSON[i] = []byte{'"', byte(i), '"'}
	}
}

// IsTradingHalt returns true if this is a trading halt status.
func (c SIPStatusCode) IsTradingHalt() bool {
	return c == StatusCodeTradingHaltCTA || c == StatusCodeTradingHaltUTP
}

// IsResume returns true if this is a trading resume/resumption status.
func (c SIPStatusCode) IsResume() bool {
	return c == StatusCodeResumeCTA || c == StatusCodeTradingResumptionUTP || c == StatusCodeQuotationResumption
}

// IsImbalance returns true if this indicates a market imbalance.
func (c SIPStatusCode) IsImbalance() bool {
	return c == StatusCodeMarketImbalanceBuy || c == StatusCodeMarketImbalanceSell ||
		c == StatusCodeMOCImbalanceBuy || c == StatusCodeMOCImbalanceSell
}

func (c SIPStatusCode) GoString() string {
	switch c {
	case StatusCodeTradingHaltCTA:
		return "StatusCodeTradingHaltCTA"
	case StatusCodeResumeCTA:
		return "StatusCodeResumeCTA"
	case StatusCodePriceIndication:
		return "StatusCodePriceIndication"
	case StatusCodeTradingRangeIndication:
		return "StatusCodeTradingRangeIndication"
	case StatusCodeMarketImbalanceBuy:
		return "StatusCodeMarketImbalanceBuy"
	case StatusCodeMarketImbalanceSell:
		return "StatusCodeMarketImbalanceSell"
	case StatusCodeMOCImbalanceBuy:
		return "StatusCodeMOCImbalanceBuy"
	case StatusCodeMOCImbalanceSell:
		return "StatusCodeMOCImbalanceSell"
	case StatusCodeNoMarketImbalance:
		return "StatusCodeNoMarketImbalance"
	case StatusCodeNoMOCImbalance:
		return "StatusCodeNoMOCImbalance"
	case StatusCodeShortSaleRestriction:
		return "StatusCodeShortSaleRestriction"
	case StatusCodeLimitUpLimitDown:
		return "StatusCodeLimitUpLimitDown"
	case StatusCodeTradingHaltUTP:
		return "StatusCodeTradingHaltUTP"
	case StatusCodeVolatilityPause:
		return "StatusCodeVolatilityPause"
	case StatusCodeQuotationResumption:
		return "StatusCodeQuotationResumption"
	case StatusCodeTradingResumptionUTP:
		return "StatusCodeTradingResumptionUTP"
	case 0:
		return "SIPStatusCode(0)"
	default:
		return fmt.Sprintf("SIPStatusCode(%c)", byte(c))
	}
}

func (c SIPStatusCode) String() string {
	return c.GoString()
}

func (c SIPStatusCode) MarshalJSON() ([]byte, error) {
	return sipStatusCodeJSON[c], nil
}

func (c *SIPStatusCode) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		*c = SIPStatusCode(data[1])
		return nil
	}
	if len(data) == 2 && data[0] == '"' && data[1] == '"' {
		*c = 0
		return nil
	}
	return fmt.Errorf("invalid SIPStatusCode: %s", data)
}
