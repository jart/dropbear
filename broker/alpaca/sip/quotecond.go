package sip

import (
	"fmt"
	"math/bits"
	"strings"
)

// QuoteCond is a bitmask of quote conditions from the SIP feed.
// Multiple conditions can be OR'd together.
type QuoteCond uint64

// Individual quote condition bits. Each corresponds to a single-character code.
const (
	QuoteCond4 QuoteCond = 1 << iota // code '4' - On Demand Intra Day Auction
	QuoteCondA                       // code 'A' - Slow Offer (CTA) / Manual Ask Auto Bid (UTP)
	QuoteCondB                       // code 'B' - Slow Bid (CTA) / Manual Bid Auto Ask (UTP)
	QuoteCondC                       // code 'C' - Closing Quote (CTA)
	QuoteCondE                       // code 'E' - Slow LRP Bid (CTA)
	QuoteCondF                       // code 'F' - Slow LRP Offer (CTA) / Fast Trading (UTP)
	QuoteCondH                       // code 'H' - Slow Bid And Offer (CTA) / Manual Bid Ask (UTP)
	QuoteCondI                       // code 'I' - Order Imbalance (UTP)
	QuoteCondL                       // code 'L' - MM Closed (CTA) / Closed Quote (UTP)
	QuoteCondN                       // code 'N' - Non Firm Quote
	QuoteCondO                       // code 'O' - Opening Quote
	QuoteCondR                       // code 'R' - MM Open (CTA) / Two Sided Open (UTP)
	QuoteCondU                       // code 'U' - Slow LRP Bid And Offer (CTA) / Manual Non Firm (UTP)
	QuoteCondW                       // code 'W' - Slow Set Slow List (CTA)
	QuoteCondX                       // code 'X' - Order Influx (UTP)
	QuoteCondY                       // code 'Y' - One Sided Open (UTP)
	QuoteCondZ                       // code 'Z' - No Open No Resume (UTP)
)

// LUT for converting character code to bit
var quoteCondFromChar [256]QuoteCond

func init() {
	quoteCondFromChar['4'] = QuoteCond4
	quoteCondFromChar['A'] = QuoteCondA
	quoteCondFromChar['B'] = QuoteCondB
	quoteCondFromChar['C'] = QuoteCondC
	quoteCondFromChar['E'] = QuoteCondE
	quoteCondFromChar['F'] = QuoteCondF
	quoteCondFromChar['H'] = QuoteCondH
	quoteCondFromChar['I'] = QuoteCondI
	quoteCondFromChar['L'] = QuoteCondL
	quoteCondFromChar['N'] = QuoteCondN
	quoteCondFromChar['O'] = QuoteCondO
	quoteCondFromChar['R'] = QuoteCondR
	quoteCondFromChar['U'] = QuoteCondU
	quoteCondFromChar['W'] = QuoteCondW
	quoteCondFromChar['X'] = QuoteCondX
	quoteCondFromChar['Y'] = QuoteCondY
	quoteCondFromChar['Z'] = QuoteCondZ
}

// LUT for converting bit back to character code
var quoteCondToChar = [64]byte{
	'4', 'A', 'B', 'C', 'E', 'F', 'H', 'I',
	'L', 'N', 'O', 'R', 'U', 'W', 'X', 'Y', 'Z',
}

// Has returns true if this condition set contains the given condition(s).
func (c QuoteCond) Has(cond QuoteCond) bool {
	return c&cond != 0
}

func (c QuoteCond) String() string {
	if c == 0 {
		return "QuoteCond(0)"
	}
	var codes []string
	for c != 0 {
		i := bits.TrailingZeros64(uint64(c))
		codes = append(codes, string(quoteCondToChar[i]))
		c &= c - 1
	}
	return strings.Join(codes, ",")
}

func (c QuoteCond) GoString() string {
	if c == 0 {
		return "0"
	}
	var parts []string
	for _, cond := range []struct {
		mask QuoteCond
		name string
	}{
		{QuoteCond4, "sip.QuoteCond4"},
		{QuoteCondA, "sip.QuoteCondA"},
		{QuoteCondB, "sip.QuoteCondB"},
		{QuoteCondC, "sip.QuoteCondC"},
		{QuoteCondE, "sip.QuoteCondE"},
		{QuoteCondF, "sip.QuoteCondF"},
		{QuoteCondH, "sip.QuoteCondH"},
		{QuoteCondI, "sip.QuoteCondI"},
		{QuoteCondL, "sip.QuoteCondL"},
		{QuoteCondN, "sip.QuoteCondN"},
		{QuoteCondO, "sip.QuoteCondO"},
		{QuoteCondR, "sip.QuoteCondR"},
		{QuoteCondU, "sip.QuoteCondU"},
		{QuoteCondW, "sip.QuoteCondW"},
		{QuoteCondX, "sip.QuoteCondX"},
		{QuoteCondY, "sip.QuoteCondY"},
		{QuoteCondZ, "sip.QuoteCondZ"},
	} {
		if c&cond.mask != 0 {
			parts = append(parts, cond.name)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("sip.QuoteCond(0x%x)", uint64(c))
	}
	return strings.Join(parts, "|")
}

func (c *QuoteCond) UnmarshalJSON(data []byte) error {
	// Parse JSON array like ["R","N"]
	if len(data) < 2 {
		return ErrInvalidQuoteCond
	}
	if data[0] == 'n' && string(data) == "null" {
		*c = 0
		return nil
	}
	if data[0] != '[' {
		return ErrInvalidQuoteCond
	}
	var result QuoteCond
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
			return ErrInvalidQuoteCond
		}
		i++
		if i >= len(data) {
			return ErrInvalidQuoteCond
		}
		ch := data[i]
		result |= quoteCondFromChar[ch]
		i++
		if i >= len(data) || data[i] != '"' {
			return ErrInvalidQuoteCond
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

func (c QuoteCond) MarshalJSON() ([]byte, error) {
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
