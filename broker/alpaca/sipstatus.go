package alpaca

import "dropbear/clocky"

// SIPStatus represents a trading status message.
type SIPStatus struct {
	Type      SIPMessageType `json:"T"`  // SIPMessageTypeStatus
	Code      SIPStatusCode  `json:"sc"` // status code
	Reason    SIPReasonCode  `json:"rc"` // reason code
	Timestamp clocky.Time    `json:"t"`  // RFC-3339 timestamp
	Symbol    string         `json:"S"`  // stock symbol
	Message   string         `json:"sm"` // status message
}

// IsTradingHalt returns true if this is a trading halt status.
func (s *SIPStatus) IsTradingHalt() bool {
	return s.Code.IsTradingHalt()
}

// IsResume returns true if this is a trading resume status.
func (s *SIPStatus) IsResume() bool {
	return s.Code.IsResume()
}

// IsCircuitBreaker returns true if this is a circuit breaker event.
func (s *SIPStatus) IsCircuitBreaker() bool {
	return s.Reason.IsCircuitBreaker()
}
