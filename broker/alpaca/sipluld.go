package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
)

// SIPLULD represents a Limit Up-Limit Down price band message.
//
// These messages are published by the SIP every 30 seconds to inform market
// participants of the current LULD price bands for a security. The bands define
// the price range within which trading is permitted.
//
// Example interpretation:
//
//	SIPLULD{Symbol:"GOOG", LowerLimit:291.74, UpperLimit:356.57, Indicator:LULDIndicatorRepublished}
//
// This means GOOG is currently trading normally. If the price tries to go below
// $291.74 or above $356.57, the NBBO will enter a "Limit State". If it stays
// there for 15 seconds, trading pauses for 5 minutes.
//
// When Indicator is A or B, the price limits are valid.
// When Indicator is C-J (limit state transitions), the price limits are zero.
type SIPLULD struct {
	Type       SIPMessageType   `json:"T"` // SIPMessageTypeLULD
	Tape       SIPTape          `json:"z"` // tape identifier
	Indicator  SIPLULDIndicator `json:"i"` // LULD indicator (see SIPLULDIndicator docs)
	Timestamp  clocky.Time      `json:"t"` // RFC-3339 timestamp
	Symbol     symbol.Symbol    `json:"S"` // stock symbol
	UpperLimit decimal.Decimal  `json:"u"` // upper price limit (zero if Indicator is C-J)
	LowerLimit decimal.Decimal  `json:"d"` // lower price limit (zero if Indicator is C-J)
}
