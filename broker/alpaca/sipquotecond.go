package alpaca

import (
	"fmt"
	"math/bits"
	"strings"
)

// SIPQuoteCond is a bitmask of quote conditions from the SIP feed.
// Multiple conditions can be OR'd together.
type SIPQuoteCond uint64

// Individual quote condition bits. Each corresponds to a single-character code.
const (
	QuoteCondOnDemandAuction SIPQuoteCond = 1 << iota // code '4' - On Demand Intra Day Auction
	QuoteCondA                                        // code 'A' - Slow Offer (CTA) / Manual Ask Auto Bid (UTP)
	QuoteCondB                                        // code 'B' - Slow Bid (CTA) / Manual Bid Auto Ask (UTP)
	QuoteCondClosing                                  // code 'C' - Closing Quote (CTA)
	QuoteCondSlowLRPBid                               // code 'E' - Slow LRP Bid (CTA)
	QuoteCondF                                        // code 'F' - Slow LRP Offer (CTA) / Fast Trading (UTP)
	QuoteCondH                                        // code 'H' - Slow Bid And Offer (CTA) / Manual Bid Ask (UTP)
	QuoteCondOrderImbalance                           // code 'I' - Order Imbalance (UTP)
	QuoteCondClosed                                   // code 'L' - MM Closed (CTA) / Closed Quote (UTP)
	QuoteCondNonFirm                                  // code 'N' - Non Firm Quote
	QuoteCondOpening                                  // code 'O' - Opening Quote
	QuoteCondR                                        // code 'R' - MM Open (CTA) / Two Sided Open (UTP)
	QuoteCondU                                        // code 'U' - Slow LRP Bid And Offer (CTA) / Manual Non Firm (UTP)
	QuoteCondSlowSetSlowList                          // code 'W' - Slow Set Slow List (CTA)
	QuoteCondOrderInflux                              // code 'X' - Order Influx (UTP)
	QuoteCondOneSidedOpen                             // code 'Y' - One Sided Open (UTP)
	QuoteCondNoOpenNoResume                           // code 'Z' - No Open No Resume (UTP)
)

// Composite condition masks for common checks
const (
	QuoteCondNonFirmMask = QuoteCondNonFirm | QuoteCondU
	QuoteCondSlowMask    = QuoteCondA | QuoteCondB | QuoteCondSlowLRPBid | QuoteCondF | QuoteCondH | QuoteCondU | QuoteCondSlowSetSlowList
)

var (
	ErrInvalidSIPQuoteCond = fmt.Errorf("invalid SIP quote condition")
)

// LUT for converting character code to bit
var quoteCondFromChar [256]SIPQuoteCond

func init() {
	quoteCondFromChar['4'] = QuoteCondOnDemandAuction
	quoteCondFromChar['A'] = QuoteCondA
	quoteCondFromChar['B'] = QuoteCondB
	quoteCondFromChar['C'] = QuoteCondClosing
	quoteCondFromChar['E'] = QuoteCondSlowLRPBid
	quoteCondFromChar['F'] = QuoteCondF
	quoteCondFromChar['H'] = QuoteCondH
	quoteCondFromChar['I'] = QuoteCondOrderImbalance
	quoteCondFromChar['L'] = QuoteCondClosed
	quoteCondFromChar['N'] = QuoteCondNonFirm
	quoteCondFromChar['O'] = QuoteCondOpening
	quoteCondFromChar['R'] = QuoteCondR
	quoteCondFromChar['U'] = QuoteCondU
	quoteCondFromChar['W'] = QuoteCondSlowSetSlowList
	quoteCondFromChar['X'] = QuoteCondOrderInflux
	quoteCondFromChar['Y'] = QuoteCondOneSidedOpen
	quoteCondFromChar['Z'] = QuoteCondNoOpenNoResume
}

// LUT for converting bit back to character code
var quoteCondToChar = [64]byte{
	'4', 'A', 'B', 'C', 'E', 'F', 'H', 'I',
	'L', 'N', 'O', 'R', 'U', 'W', 'X', 'Y', 'Z',
}

// Has returns true if this condition set contains the given condition(s).
func (c SIPQuoteCond) Has(cond SIPQuoteCond) bool {
	return c&cond != 0
}

// IsNonFirm returns true if this contains a non-firm quote condition.
func (c SIPQuoteCond) IsNonFirm() bool {
	return c&QuoteCondNonFirmMask != 0
}

// IsSlow returns true if this contains a slow quote condition.
func (c SIPQuoteCond) IsSlow() bool {
	return c&QuoteCondSlowMask != 0
}

func (c SIPQuoteCond) String() string {
	if c == 0 {
		return "SIPQuoteCond(0)"
	}
	var codes []string
	for c != 0 {
		i := bits.TrailingZeros64(uint64(c))
		codes = append(codes, string(quoteCondToChar[i]))
		c &= c - 1
	}
	return strings.Join(codes, ",")
}

func (c SIPQuoteCond) GoString() string {
	return fmt.Sprintf("SIPQuoteCond(0x%x)", uint64(c))
}

func (c *SIPQuoteCond) UnmarshalJSON(data []byte) error {
	// Parse JSON array like ["R","N"]
	if len(data) < 2 {
		return ErrInvalidSIPQuoteCond
	}
	if data[0] == 'n' && string(data) == "null" {
		*c = 0
		return nil
	}
	if data[0] != '[' {
		return ErrInvalidSIPQuoteCond
	}
	var result SIPQuoteCond
	i := 1
	for i < len(data) {
		// Skip whitespace and commas
		for i < len(data) && jsonArraySkip[data[i]] {
			i++
		}
		if i >= len(data) || data[i] == ']' {
			break
		}
		// Expect "X"
		if data[i] != '"' {
			return ErrInvalidSIPQuoteCond
		}
		i++
		if i >= len(data) {
			return ErrInvalidSIPQuoteCond
		}
		ch := data[i]
		result |= quoteCondFromChar[ch]
		i++
		if i >= len(data) || data[i] != '"' {
			return ErrInvalidSIPQuoteCond
		}
		i++
	}
	*c = result
	return nil
}

// Pre-computed JSON for empty and single-condition cases
var quoteCondJSONEmpty = []byte("[]")
var quoteCondJSONSingle [64][]byte

func init() {
	for i := range 17 {
		quoteCondJSONSingle[i] = []byte{'[', '"', quoteCondToChar[i], '"', ']'}
	}
}

func (c SIPQuoteCond) MarshalJSON() ([]byte, error) {
	if c == 0 {
		return quoteCondJSONEmpty, nil
	}
	// Fast path for single condition (common case) - uses tzcnt instruction
	if c&(c-1) == 0 {
		return quoteCondJSONSingle[bits.TrailingZeros64(uint64(c))], nil
	}
	// Multiple conditions - iterate only set bits
	buf := make([]byte, 0, 64)
	buf = append(buf, '[')
	first := true
	for c != 0 {
		i := bits.TrailingZeros64(uint64(c))
		if !first {
			buf = append(buf, ',')
		}
		buf = append(buf, '"', quoteCondToChar[i], '"')
		first = false
		c &= c - 1 // clear lowest set bit
	}
	buf = append(buf, ']')
	return buf, nil
}
