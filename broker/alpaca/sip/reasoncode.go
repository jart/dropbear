package sip

import (
	"fmt"
)

// ReasonCode represents a halt/resume reason code from the SIP feed.
// The value is the ASCII string packed into a uint32 (little-endian).
type ReasonCode uint32

// Reason codes - ASCII characters packed into uint32
const (

	// Tape A & B
	ReasonCodeNewsReleased ReasonCode = 'D' // "D" - News Released
	ReasonCodeOrderImb     ReasonCode = 'I' // "I" - Order Imbalance
	ReasonCodeLULD         ReasonCode = 'M' // "M" - Limit Up-Limit Down Trading Pause
	ReasonCodeNewsPending  ReasonCode = 'P' // "P" - News Pending
	ReasonCodeOperational  ReasonCode = 'X' // "X" - Operational
	ReasonCodeSubPenny     ReasonCode = 'Y' // "Y" - Sub-Penny Trading
	ReasonCodeCircuitLvl1  ReasonCode = '1' // "1" - Market-Wide Circuit Breaker Level 1 ($SPX dipped 7% drink a coffee)
	ReasonCodeCircuitLvl2  ReasonCode = '2' // "2" - Market-Wide Circuit Breaker Level 2 ($SPX dipped 13% go have a smoke)
	ReasonCodeCircuitLvl3  ReasonCode = '3' // "3" - Market-Wide Circuit Breaker Level 3 ($SPX dipped 20% go home and sleep)

	// Tape C & O
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
	ReasonCodeHaltOtherRegAuth   ReasonCode = 'C' | '1'<<8 | '1'<<16           // "C11" - Trade Halt Concluded By Other Regulatory Auth
	ReasonCodeO1                 ReasonCode = 'O' | '1'<<8                     // "O1" - Operations Halt, Contact Market Operations
	ReasonCodeT3                 ReasonCode = 'T' | '3'<<8                     // "T3" - News and Resumption Times
	ReasonCodeT6                 ReasonCode = 'T' | '6'<<8                     // "T6" - Regulatory Halt Extraordinary Market Activity
	ReasonCodeT7                 ReasonCode = 'T' | '7'<<8                     // "T7" - Single Stock Trading Pause/Quotation-Only Period
	ReasonCodeT8                 ReasonCode = 'T' | '8'<<8                     // "T8" - Halt ETF
	ReasonCodeR4                 ReasonCode = 'R' | '4'<<8                     // "R4" - Qualifications Issues Reviewed/Resolved; Quotations/Trading to Resume
	ReasonCodeR9                 ReasonCode = 'R' | '9'<<8                     // "R9" - Filing Requirements Satisfied/Resolved; Quotations/Trading To Resume
	ReasonCodeC3                 ReasonCode = 'C' | '3'<<8                     // "C3" - Issuer News Not Forthcoming; Quotations/Trading To Resume
	ReasonCodeC4                 ReasonCode = 'C' | '4'<<8                     // "C4" - Qualifications Halt ended; maint. Req. met; Resume
	ReasonCodeC9                 ReasonCode = 'C' | '9'<<8                     // "C9" - Qualifications Halt Concluded; Filings Met; Quotes/Trades To Resume
	ReasonCodeMWCQ               ReasonCode = 'M' | 'W'<<8 | 'C'<<16 | 'Q'<<24 // "MWCQ" - Market Wide Circuit Breaker Resumption
	ReasonCodeMWC0               ReasonCode = 'M' | 'W'<<8 | 'C'<<16 | '0'<<24 // "MWC0" - Market Wide Circuit Breaker Halt – Carry over from previous day
	ReasonCodeMWC1               ReasonCode = 'M' | 'W'<<8 | 'C'<<16 | '1'<<24 // "MWC1" - Market Wide Circuit Breaker Halt – Level 1 ($SPX dipped 7% drink a coffee)
	ReasonCodeMWC2               ReasonCode = 'M' | 'W'<<8 | 'C'<<16 | '2'<<24 // "MWC2" - Market Wide Circuit Breaker Halt – Level 2 ($SPX dipped 13% go have a smoke)
	ReasonCodeMWC3               ReasonCode = 'M' | 'W'<<8 | 'C'<<16 | '3'<<24 // "MWC3" - Market Wide Circuit Breaker Halt – Level 3 ($SPX dipped 20% go home and sleep)
)

// ParseReasonCode converts a byte slice to a ReasonCode.
func ParseReasonCode(b []byte) (ReasonCode, error) {
	var rc ReasonCode
	switch len(b) {
	case 0:
		return 0, nil
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
	return rc, nil
}

// IsKnown returns true if this is a known reason code.
func (rc ReasonCode) IsKnown() bool {
	switch rc {
	case 0, ReasonCodeNewsReleased, ReasonCodeOrderImb, ReasonCodeLULD,
		ReasonCodeNewsPending, ReasonCodeOperational, ReasonCodeSubPenny,
		ReasonCodeCircuitLvl1, ReasonCodeCircuitLvl2, ReasonCodeCircuitLvl3,
		ReasonCodeHaltNewsPending, ReasonCodeHaltNewsDissem, ReasonCodeHaltSECOrderSIP,
		ReasonCodeHaltInfoRequested, ReasonCodeHaltNonCompliance, ReasonCodeHaltFilingsNotCurr,
		ReasonCodeHaltSECOrderNotSIP, ReasonCodeHaltRegConcern, ReasonCodeIPONotYetTrading,
		ReasonCodeCorporateAction, ReasonCodeQuotationNotAvail, ReasonCodeLULDPause,
		ReasonCodeHaltOtherRegAuth, ReasonCodeMWCQ, ReasonCodeT3, ReasonCodeT7, ReasonCodeR4,
		ReasonCodeR9, ReasonCodeC3, ReasonCodeC4, ReasonCodeC9:
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
		ReasonCodeLULDPause, ReasonCodeCircuitLvl1, ReasonCodeCircuitLvl2, ReasonCodeCircuitLvl3,
		ReasonCodeT6, ReasonCodeT7, ReasonCodeT8, ReasonCodeMWC0, ReasonCodeMWC1, ReasonCodeMWC2,
		ReasonCodeMWC3, ReasonCodeO1:
		return true
	}
	return false
}

// IsResume returns true if this reason code indicates a resumption of trading.
func (rc ReasonCode) IsResume() bool {
	switch rc {
	case ReasonCodeNewsReleased, ReasonCodeHaltOtherRegAuth, ReasonCodeT3, ReasonCodeT7,
		ReasonCodeR4, ReasonCodeR9, ReasonCodeC3, ReasonCodeC4, ReasonCodeC9, ReasonCodeMWCQ:
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
	case ReasonCodeHaltOtherRegAuth:
		return "sip.ReasonCodeHaltOtherRegAuth"
	case ReasonCodeT3:
		return "sip.ReasonCodeT3"
	case ReasonCodeT7:
		return "sip.ReasonCodeT7"
	case ReasonCodeR4:
		return "sip.ReasonCodeR4"
	case ReasonCodeR9:
		return "sip.ReasonCodeR9"
	case ReasonCodeC3:
		return "sip.ReasonCodeC3"
	case ReasonCodeC4:
		return "sip.ReasonCodeC4"
	case ReasonCodeC9:
		return "sip.ReasonCodeC9"
	case ReasonCodeMWCQ:
		return "sip.ReasonCodeMWCQ"
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
