package sip

import (
	"fmt"
)

// ReasonCode represents a halt/resume reason code from the SIP feed.
// The value is the ASCII string packed into a uint32 (little-endian).
type ReasonCode uint32

// Reason codes - ASCII characters packed into uint32
const (
	ReasonCodeNewsReleased       ReasonCode = 'D'                              // "D" - News Released
	ReasonCodeOrderImb           ReasonCode = 'I'                              // "I" - Order Imbalance
	ReasonCodeLULD               ReasonCode = 'M'                              // "M" - Limit Up-Limit Down Trading Pause
	ReasonCodeNewsPending        ReasonCode = 'P'                              // "P" - News Pending
	ReasonCodeOperational        ReasonCode = 'X'                              // "X" - Operational
	ReasonCodeSubPenny           ReasonCode = 'Y'                              // "Y" - Sub-Penny Trading
	ReasonCodeCircuitLvl1        ReasonCode = '1'                              // "1" - Market-Wide Circuit Breaker Level 1 ($SPX dipped 7% drink a coffee)
	ReasonCodeCircuitLvl2        ReasonCode = '2'                              // "2" - Market-Wide Circuit Breaker Level 2 ($SPX dipped 13% go have a smoke)
	ReasonCodeCircuitLvl3        ReasonCode = '3'                              // "3" - Market-Wide Circuit Breaker Level 3 ($SPX dipped 20% go home and sleep)
	ReasonCodeHaltNewsPending    ReasonCode = 'T' | '1'<<8                     // "T1" - Halt News Pending
	ReasonCodeHaltNewsDissem     ReasonCode = 'T' | '2'<<8                     // "T2" - Halt News Dissemination
	ReasonCodeHaltSECOrderSIP    ReasonCode = 'T' | '5'<<8                     // "T5" - Single Stock Trading Pause
	ReasonCodeHaltInfoRequested  ReasonCode = 'T' | '1'<<8 | '2'<<16           // "T12" - Trading Halted; Info Requested
	ReasonCodeHaltNonCompliance  ReasonCode = 'H' | '4'<<8                     // "H4" - Halt Non Compliance
	ReasonCodeHaltFilingsNotCurr ReasonCode = 'H' | '9'<<8                     // "H9" - Halt Filings Not Current
	ReasonCodeHaltSECOrderNotSIP ReasonCode = 'H' | '1'<<8 | '0'<<16           // "H10" - SEC Trading Suspension
	ReasonCodeHaltRegConcern     ReasonCode = 'H' | '1'<<8 | '1'<<16           // "H11" - Halt Regulatory Concern
	ReasonCodeIPONotYetTrading   ReasonCode = 'I' | 'P'<<8 | 'O'<<16 | '1'<<24 // "IPO1" - IPO Issue not yet Trading
	ReasonCodeCorporateAction    ReasonCode = 'M' | '1'<<8                     // "M1" - Corporate Action
	ReasonCodeQuotationNotAvail  ReasonCode = 'M' | '2'<<8                     // "M2" - Quotation Not Available
	ReasonCodeLULDPause          ReasonCode = 'L' | 'U'<<8 | 'D'<<16 | 'P'<<24 // "LUDP" - Limit Up-Limit Down Pause
	ReasonCodeVolatilityPause    ReasonCode = 'M' | 'W'<<8 | 'C'<<16 | 'B'<<24 // "MWCB" - Market Wide Circuit Breaker
)

// ParseReasonCode converts a byte slice to a ReasonCode.
func ParseReasonCode(b []byte) (ReasonCode, error) {
	var rc ReasonCode
	switch len(b) {
	case 1:
		rc = ReasonCode(b[0])
	case 2:
		rc = ReasonCode(b[0]) | ReasonCode(b[1])<<8
	case 3:
		rc = ReasonCode(b[0]) | ReasonCode(b[1])<<8 | ReasonCode(b[2])<<16
	case 4:
		rc = ReasonCode(b[0]) | ReasonCode(b[1])<<8 | ReasonCode(b[2])<<16 | ReasonCode(b[3])<<24
	default:
		return 0, ErrInvalidReasonCode
	}
	if !rc.IsValid() {
		return 0, ErrInvalidReasonCode
	}
	return rc, nil
}

// IsValid returns true if this is a known reason code.
func (rc ReasonCode) IsValid() bool {
	switch rc {
	case ReasonCodeNewsReleased, ReasonCodeOrderImb, ReasonCodeLULD,
		ReasonCodeNewsPending, ReasonCodeOperational, ReasonCodeSubPenny,
		ReasonCodeCircuitLvl1, ReasonCodeCircuitLvl2, ReasonCodeCircuitLvl3,
		ReasonCodeHaltNewsPending, ReasonCodeHaltNewsDissem, ReasonCodeHaltSECOrderSIP,
		ReasonCodeHaltInfoRequested, ReasonCodeHaltNonCompliance, ReasonCodeHaltFilingsNotCurr,
		ReasonCodeHaltSECOrderNotSIP, ReasonCodeHaltRegConcern, ReasonCodeIPONotYetTrading,
		ReasonCodeCorporateAction, ReasonCodeQuotationNotAvail, ReasonCodeLULDPause,
		ReasonCodeVolatilityPause:
		return true
	}
	return false
}

// IsHalt returns true if this reason code indicates a trading halt.
func (rc ReasonCode) IsHalt() bool {
	switch rc {
	case ReasonCodeHaltNewsPending, ReasonCodeHaltNewsDissem, ReasonCodeHaltSECOrderSIP,
		ReasonCodeHaltInfoRequested, ReasonCodeHaltNonCompliance, ReasonCodeHaltFilingsNotCurr,
		ReasonCodeHaltSECOrderNotSIP, ReasonCodeHaltRegConcern, ReasonCodeIPONotYetTrading,
		ReasonCodeLULDPause, ReasonCodeVolatilityPause,
		ReasonCodeCircuitLvl1, ReasonCodeCircuitLvl2, ReasonCodeCircuitLvl3:
		return true
	}
	return false
}

// IsCircuitBreaker returns true if this is a market-wide circuit breaker.
func (rc ReasonCode) IsCircuitBreaker() bool {
	switch rc {
	case ReasonCodeCircuitLvl1, ReasonCodeCircuitLvl2, ReasonCodeCircuitLvl3,
		ReasonCodeVolatilityPause:
		return true
	}
	return false
}

func (rc ReasonCode) String() string {
	var buf [4]byte
	n := 0
	for i := 0; i < 4; i++ {
		b := byte(rc >> (i * 8))
		if b == 0 {
			break
		}
		buf[i] = b
		n++
	}
	return string(buf[:n])
}

func (rc ReasonCode) GoString() string {
	switch rc {
	case ReasonCodeNewsReleased:
		return "sip.ReasonCodeNewsReleased"
	case ReasonCodeOrderImb:
		return "sip.ReasonCodeOrderImb"
	case ReasonCodeLULD:
		return "sip.ReasonCodeLULD"
	case ReasonCodeNewsPending:
		return "sip.ReasonCodeNewsPending"
	case ReasonCodeOperational:
		return "sip.ReasonCodeOperational"
	case ReasonCodeSubPenny:
		return "sip.ReasonCodeSubPenny"
	case ReasonCodeCircuitLvl1:
		return "sip.ReasonCodeCircuitLvl1"
	case ReasonCodeCircuitLvl2:
		return "sip.ReasonCodeCircuitLvl2"
	case ReasonCodeCircuitLvl3:
		return "sip.ReasonCodeCircuitLvl3"
	case ReasonCodeHaltNewsPending:
		return "sip.ReasonCodeHaltNewsPending"
	case ReasonCodeHaltNewsDissem:
		return "sip.ReasonCodeHaltNewsDissem"
	case ReasonCodeHaltSECOrderSIP:
		return "sip.ReasonCodeHaltSECOrderSIP"
	case ReasonCodeHaltInfoRequested:
		return "sip.ReasonCodeHaltInfoRequested"
	case ReasonCodeHaltNonCompliance:
		return "sip.ReasonCodeHaltNonCompliance"
	case ReasonCodeHaltFilingsNotCurr:
		return "sip.ReasonCodeHaltFilingsNotCurr"
	case ReasonCodeHaltSECOrderNotSIP:
		return "sip.ReasonCodeHaltSECOrderNotSIP"
	case ReasonCodeHaltRegConcern:
		return "sip.ReasonCodeHaltRegConcern"
	case ReasonCodeIPONotYetTrading:
		return "sip.ReasonCodeIPONotYetTrading"
	case ReasonCodeCorporateAction:
		return "sip.ReasonCodeCorporateAction"
	case ReasonCodeQuotationNotAvail:
		return "sip.ReasonCodeQuotationNotAvail"
	case ReasonCodeLULDPause:
		return "sip.ReasonCodeLULDPause"
	case ReasonCodeVolatilityPause:
		return "sip.ReasonCodeVolatilityPause"
	default:
		return fmt.Sprintf("sip.ReasonCode(0x%x)", uint32(rc))
	}
}

func (rc ReasonCode) MarshalJSON() ([]byte, error) {
	s := rc.String()
	buf := make([]byte, len(s)+2)
	buf[0] = '"'
	copy(buf[1:], s)
	buf[len(buf)-1] = '"'
	return buf, nil
}

func (rc *ReasonCode) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return ErrInvalidReasonCode
	}
	s := data[1 : len(data)-1]
	code, err := ParseReasonCode(s)
	if err != nil {
		return err
	}
	*rc = code
	return nil
}
