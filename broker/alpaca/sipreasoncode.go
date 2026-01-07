package alpaca

import (
	"errors"
	"fmt"
)

var ErrUnknownReasonCode = errors.New("unknown SIP reason code")

// SIPReasonCode represents a halt/resume reason code from the SIP feed.
type SIPReasonCode uint8

// Reason codes - arbitrary numbers mapped to actual string codes
const (
	ReasonCodeUnknown            SIPReasonCode = iota
	ReasonCodeNewsReleased                     // "D" - News Released
	ReasonCodeOrderImb                         // "I" - Order Imbalance
	ReasonCodeLULD                             // "M" - Limit Up-Limit Down Trading Pause
	ReasonCodeNewsPending                      // "P" - News Pending
	ReasonCodeOperational                      // "X" - Operational
	ReasonCodeSubPenny                         // "Y" - Sub-Penny Trading
	ReasonCodeCircuitLvl1                      // "1" - Market-Wide Circuit Breaker Level 1
	ReasonCodeCircuitLvl2                      // "2" - Market-Wide Circuit Breaker Level 2
	ReasonCodeCircuitLvl3                      // "3" - Market-Wide Circuit Breaker Level 3
	ReasonCodeHaltNewsPending                  // "T1" - Halt News Pending
	ReasonCodeHaltNewsDissem                   // "T2" - Halt News Dissemination
	ReasonCodeHaltSECOrderSIP                  // "T5" - Single Stock Trading Pause
	ReasonCodeHaltInfoRequested                // "T12" - Trading Halted; Info Requested
	ReasonCodeHaltNonCompliance                // "H4" - Halt Non Compliance
	ReasonCodeHaltFilingsNotCurr               // "H9" - Halt Filings Not Current
	ReasonCodeHaltSECOrderNotSIP               // "H10" - SEC Trading Suspension
	ReasonCodeHaltRegConcern                   // "H11" - Halt Regulatory Concern
	ReasonCodeIPONotYetTrading                 // "IPO1" - IPO Issue not yet Trading
	ReasonCodeCorporateAction                  // "M1" - Corporate Action
	ReasonCodeQuotationNotAvail                // "M2" - Quotation Not Available
	ReasonCodeLULDPause                        // "LUDP" - Limit Up-Limit Down Pause
	ReasonCodeVolatilityPause                  // "MWCB" - Market Wide Circuit Breaker
	reasonCodeCount
)

// LUT for string -> code (populated in init)
var reasonCodeFromString = map[string]SIPReasonCode{
	"D": ReasonCodeNewsReleased, "I": ReasonCodeOrderImb, "M": ReasonCodeLULD,
	"P": ReasonCodeNewsPending, "X": ReasonCodeOperational, "Y": ReasonCodeSubPenny,
	"1": ReasonCodeCircuitLvl1, "2": ReasonCodeCircuitLvl2, "3": ReasonCodeCircuitLvl3,
	"T1": ReasonCodeHaltNewsPending, "T2": ReasonCodeHaltNewsDissem,
	"T5": ReasonCodeHaltSECOrderSIP, "T12": ReasonCodeHaltInfoRequested,
	"H4": ReasonCodeHaltNonCompliance, "H9": ReasonCodeHaltFilingsNotCurr,
	"H10": ReasonCodeHaltSECOrderNotSIP, "H11": ReasonCodeHaltRegConcern,
	"IPO1": ReasonCodeIPONotYetTrading, "M1": ReasonCodeCorporateAction,
	"M2": ReasonCodeQuotationNotAvail, "LUDP": ReasonCodeLULDPause,
	"MWCB": ReasonCodeVolatilityPause,
}

// LUT for code -> string
var reasonCodeToString = [reasonCodeCount]string{
	ReasonCodeUnknown:            "",
	ReasonCodeNewsReleased:       "D",
	ReasonCodeOrderImb:           "I",
	ReasonCodeLULD:               "M",
	ReasonCodeNewsPending:        "P",
	ReasonCodeOperational:        "X",
	ReasonCodeSubPenny:           "Y",
	ReasonCodeCircuitLvl1:        "1",
	ReasonCodeCircuitLvl2:        "2",
	ReasonCodeCircuitLvl3:        "3",
	ReasonCodeHaltNewsPending:    "T1",
	ReasonCodeHaltNewsDissem:     "T2",
	ReasonCodeHaltSECOrderSIP:    "T5",
	ReasonCodeHaltInfoRequested:  "T12",
	ReasonCodeHaltNonCompliance:  "H4",
	ReasonCodeHaltFilingsNotCurr: "H9",
	ReasonCodeHaltSECOrderNotSIP: "H10",
	ReasonCodeHaltRegConcern:     "H11",
	ReasonCodeIPONotYetTrading:   "IPO1",
	ReasonCodeCorporateAction:    "M1",
	ReasonCodeQuotationNotAvail:  "M2",
	ReasonCodeLULDPause:          "LUDP",
	ReasonCodeVolatilityPause:    "MWCB",
}

// Pre-computed JSON for marshaling
var reasonCodeJSON [reasonCodeCount][]byte

func init() {
	for i := range reasonCodeCount {
		s := reasonCodeToString[i]
		if s == "" {
			reasonCodeJSON[i] = []byte(`""`)
		} else {
			reasonCodeJSON[i] = []byte(`"` + s + `"`)
		}
	}
}

// ParseReasonCode converts a byte slice to a SIPReasonCode.
// Returns ErrUnknownReasonCode if the code is not recognized.
func ParseReasonCode(b []byte) (SIPReasonCode, error) {
	if len(b) == 0 {
		return ReasonCodeUnknown, nil // empty is valid (no reason)
	}
	if rc, ok := reasonCodeFromString[string(b)]; ok {
		return rc, nil
	}
	return ReasonCodeUnknown, ErrUnknownReasonCode
}

// IsHalt returns true if this reason code indicates a trading halt.
var reasonCodeIsHalt = [reasonCodeCount]bool{
	ReasonCodeHaltNewsPending:    true,
	ReasonCodeHaltNewsDissem:     true,
	ReasonCodeHaltSECOrderSIP:    true,
	ReasonCodeHaltInfoRequested:  true,
	ReasonCodeHaltNonCompliance:  true,
	ReasonCodeHaltFilingsNotCurr: true,
	ReasonCodeHaltSECOrderNotSIP: true,
	ReasonCodeHaltRegConcern:     true,
	ReasonCodeIPONotYetTrading:   true,
	ReasonCodeLULDPause:          true,
	ReasonCodeVolatilityPause:    true,
	ReasonCodeCircuitLvl1:        true,
	ReasonCodeCircuitLvl2:        true,
	ReasonCodeCircuitLvl3:        true,
}

func (rc SIPReasonCode) IsHalt() bool {
	return rc < reasonCodeCount && reasonCodeIsHalt[rc]
}

// IsCircuitBreaker returns true if this is a market-wide circuit breaker.
var reasonCodeIsCircuitBreaker = [reasonCodeCount]bool{
	ReasonCodeCircuitLvl1:     true,
	ReasonCodeCircuitLvl2:     true,
	ReasonCodeCircuitLvl3:     true,
	ReasonCodeVolatilityPause: true,
}

func (rc SIPReasonCode) IsCircuitBreaker() bool {
	return rc < reasonCodeCount && reasonCodeIsCircuitBreaker[rc]
}

func (rc SIPReasonCode) String() string {
	if rc < reasonCodeCount {
		return reasonCodeToString[rc]
	}
	return fmt.Sprintf("alpaca.SIPReasonCode(%d)", rc)
}

func (rc SIPReasonCode) GoString() string {
	switch rc {
	case ReasonCodeUnknown:
		return "alpaca.ReasonCodeUnknown"
	case ReasonCodeNewsReleased:
		return "alpaca.ReasonCodeNewsReleased"
	case ReasonCodeOrderImb:
		return "alpaca.ReasonCodeOrderImb"
	case ReasonCodeLULD:
		return "alpaca.ReasonCodeLULD"
	case ReasonCodeNewsPending:
		return "alpaca.ReasonCodeNewsPending"
	case ReasonCodeOperational:
		return "alpaca.ReasonCodeOperational"
	case ReasonCodeSubPenny:
		return "alpaca.ReasonCodeSubPenny"
	case ReasonCodeCircuitLvl1:
		return "alpaca.ReasonCodeCircuitLvl1"
	case ReasonCodeCircuitLvl2:
		return "alpaca.ReasonCodeCircuitLvl2"
	case ReasonCodeCircuitLvl3:
		return "alpaca.ReasonCodeCircuitLvl3"
	case ReasonCodeHaltNewsPending:
		return "alpaca.ReasonCodeHaltNewsPending"
	case ReasonCodeHaltNewsDissem:
		return "alpaca.ReasonCodeHaltNewsDissem"
	case ReasonCodeHaltSECOrderSIP:
		return "alpaca.ReasonCodeHaltSECOrderSIP"
	case ReasonCodeHaltInfoRequested:
		return "alpaca.ReasonCodeHaltInfoRequested"
	case ReasonCodeHaltNonCompliance:
		return "alpaca.ReasonCodeHaltNonCompliance"
	case ReasonCodeHaltFilingsNotCurr:
		return "alpaca.ReasonCodeHaltFilingsNotCurr"
	case ReasonCodeHaltSECOrderNotSIP:
		return "alpaca.ReasonCodeHaltSECOrderNotSIP"
	case ReasonCodeHaltRegConcern:
		return "alpaca.ReasonCodeHaltRegConcern"
	case ReasonCodeIPONotYetTrading:
		return "alpaca.ReasonCodeIPONotYetTrading"
	case ReasonCodeCorporateAction:
		return "alpaca.ReasonCodeCorporateAction"
	case ReasonCodeQuotationNotAvail:
		return "alpaca.ReasonCodeQuotationNotAvail"
	case ReasonCodeLULDPause:
		return "alpaca.ReasonCodeLULDPause"
	case ReasonCodeVolatilityPause:
		return "alpaca.ReasonCodeVolatilityPause"
	default:
		return fmt.Sprintf("alpaca.SIPReasonCode(%d)", rc)
	}
}

func (rc SIPReasonCode) MarshalJSON() ([]byte, error) {
	if rc < reasonCodeCount {
		return reasonCodeJSON[rc], nil
	}
	return []byte(`""`), nil
}

func (rc *SIPReasonCode) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid SIPReasonCode: %s", data)
	}
	s := data[1 : len(data)-1]
	if len(s) == 0 {
		*rc = ReasonCodeUnknown
		return nil
	}
	if code, ok := reasonCodeFromString[string(s)]; ok {
		*rc = code
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnknownReasonCode, s)
}
