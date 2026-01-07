package alpaca

import (
	"fmt"
	"math/bits"
	"strings"
)

// SIPTradeCond is a bitmask of trade conditions from the SIP feed.
// Multiple conditions can be OR'd together.
type SIPTradeCond uint64

// Individual trade condition bits. Each corresponds to a single-character code.
const (
	TradeCondRegularSaleCTA     SIPTradeCond = 1 << iota // code ' ' - Regular Sale (CTA)
	TradeCondRegularSale                                 // code '@' - Regular Sale (UTP)
	TradeCondStoppedStock                                // code '1' - Stopped Stock (UTP)
	TradeCondDerivativelyPriced                          // code '4' - Derivatively Priced
	TradeCondReopening                                   // code '5' - Reopening Trade/Prints
	TradeCondClosing                                     // code '6' - Closing Trade/Prints
	TradeCondQCT                                         // code '7' - Qualified Contingent Trade
	TradeCondReserved                                    // code '8' - Reserved (CTA) / 611 Exempt (UTP)
	TradeCondCorrectedClose                              // code '9' - Corrected Consolidated Close
	TradeCondAcquisition                                 // code 'A' - Acquisition (UTP)
	TradeCondB                                           // code 'B' - Average Price (CTA) / Bunched (UTP)
	TradeCondCash                                        // code 'C' - Cash Trade/Sale
	TradeCondDistribution                                // code 'D' - Distribution (UTP)
	TradeCondE                                           // code 'E' - Auto Execution (CTA) / Placeholder (UTP)
	TradeCondISO                                         // code 'F' - Intermarket Sweep Order
	TradeCondBunchedSold                                 // code 'G' - Bunched Sold (UTP)
	TradeCondPriceVariation                              // code 'H' - Price Variation Trade
	TradeCondOddLot                                      // code 'I' - Odd Lot Trade
	TradeCondRule127                                     // code 'K' - Rule 127/155
	TradeCondSoldLast                                    // code 'L' - Sold Last
	TradeCondOfficialClose                               // code 'M' - Market Center Official Close
	TradeCondNextDay                                     // code 'N' - Next Day Trade
	TradeCondOpening                                     // code 'O' - Opening Trade/Prints
	TradeCondPriorRefPrice                               // code 'P' - Prior Reference Price
	TradeCondOfficialOpen                                // code 'Q' - Market Center Official Open
	TradeCondSeller                                      // code 'R' - Seller
	TradeCondSplit                                       // code 'S' - Split Trade (UTP)
	TradeCondExtendedHours                               // code 'T' - Extended Hours / Form T
	TradeCondExtendedHoursOOS                            // code 'U' - Extended Hours Sold (Out of Sequence)
	TradeCondContingent                                  // code 'V' - Contingent Trade
	TradeCondAveragePriceUTP                             // code 'W' - Average Price Trade (UTP)
	TradeCondCross                                       // code 'X' - Cross Trade
	TradeCondYellowFlag                                  // code 'Y' - Yellow Flag Regular Trade (UTP)
	TradeCondSoldOOS                                     // code 'Z' - Sold (Out of Sequence)
)

// Composite condition masks for common checks
const (
	TradeCondRegularSaleMask   = TradeCondRegularSaleCTA | TradeCondRegularSale
	TradeCondOddLotMask        = TradeCondOddLot
	TradeCondExtendedHoursMask = TradeCondExtendedHours | TradeCondExtendedHoursOOS
)

var (
	ErrInvalidSIPTradeCond = fmt.Errorf("invalid SIP trade condition")
)

// LUT for converting character code to bit
var tradeCondFromChar [256]SIPTradeCond

// LUT for JSON array parsing - skip whitespace and commas
var jsonArraySkip [256]bool

func init() {
	jsonArraySkip[' '] = true
	jsonArraySkip[','] = true
	jsonArraySkip['\t'] = true
	jsonArraySkip['\n'] = true
	jsonArraySkip['\r'] = true
}

func init() {
	tradeCondFromChar[' '] = TradeCondRegularSaleCTA
	tradeCondFromChar['@'] = TradeCondRegularSale
	tradeCondFromChar['1'] = TradeCondStoppedStock
	tradeCondFromChar['4'] = TradeCondDerivativelyPriced
	tradeCondFromChar['5'] = TradeCondReopening
	tradeCondFromChar['6'] = TradeCondClosing
	tradeCondFromChar['7'] = TradeCondQCT
	tradeCondFromChar['8'] = TradeCondReserved
	tradeCondFromChar['9'] = TradeCondCorrectedClose
	tradeCondFromChar['A'] = TradeCondAcquisition
	tradeCondFromChar['B'] = TradeCondB
	tradeCondFromChar['C'] = TradeCondCash
	tradeCondFromChar['D'] = TradeCondDistribution
	tradeCondFromChar['E'] = TradeCondE
	tradeCondFromChar['F'] = TradeCondISO
	tradeCondFromChar['G'] = TradeCondBunchedSold
	tradeCondFromChar['H'] = TradeCondPriceVariation
	tradeCondFromChar['I'] = TradeCondOddLot
	tradeCondFromChar['K'] = TradeCondRule127
	tradeCondFromChar['L'] = TradeCondSoldLast
	tradeCondFromChar['M'] = TradeCondOfficialClose
	tradeCondFromChar['N'] = TradeCondNextDay
	tradeCondFromChar['O'] = TradeCondOpening
	tradeCondFromChar['P'] = TradeCondPriorRefPrice
	tradeCondFromChar['Q'] = TradeCondOfficialOpen
	tradeCondFromChar['R'] = TradeCondSeller
	tradeCondFromChar['S'] = TradeCondSplit
	tradeCondFromChar['T'] = TradeCondExtendedHours
	tradeCondFromChar['U'] = TradeCondExtendedHoursOOS
	tradeCondFromChar['V'] = TradeCondContingent
	tradeCondFromChar['W'] = TradeCondAveragePriceUTP
	tradeCondFromChar['X'] = TradeCondCross
	tradeCondFromChar['Y'] = TradeCondYellowFlag
	tradeCondFromChar['Z'] = TradeCondSoldOOS
}

// LUT for converting bit back to character code
var tradeCondToChar = [64]byte{
	' ', '@', '1', '4', '5', '6', '7', '8', '9',
	'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I',
	'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S',
	'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
}

// Has returns true if this condition set contains the given condition(s).
func (c SIPTradeCond) Has(cond SIPTradeCond) bool {
	return c&cond != 0
}

// IsRegularSale returns true if this contains a regular sale condition.
func (c SIPTradeCond) IsRegularSale() bool {
	return c&TradeCondRegularSaleMask != 0
}

// IsOddLot returns true if this contains an odd lot condition.
func (c SIPTradeCond) IsOddLot() bool {
	return c&TradeCondOddLotMask != 0
}

// IsExtendedHours returns true if this contains an extended hours condition.
func (c SIPTradeCond) IsExtendedHours() bool {
	return c&TradeCondExtendedHoursMask != 0
}

func (c SIPTradeCond) String() string {
	if c == 0 {
		return "SIPTradeCond(0)"
	}
	var codes []string
	for c != 0 {
		i := bits.TrailingZeros64(uint64(c))
		codes = append(codes, string(tradeCondToChar[i]))
		c &= c - 1
	}
	return strings.Join(codes, ",")
}

func (c SIPTradeCond) GoString() string {
	return fmt.Sprintf("SIPTradeCond(0x%x)", uint64(c))
}

func (c *SIPTradeCond) UnmarshalJSON(data []byte) error {
	// Parse JSON array like ["@","F"]
	if len(data) < 2 {
		return ErrInvalidSIPTradeCond
	}
	if data[0] == 'n' && string(data) == "null" {
		*c = 0
		return nil
	}
	if data[0] != '[' {
		return ErrInvalidSIPTradeCond
	}
	var result SIPTradeCond
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
			return ErrInvalidSIPTradeCond
		}
		i++
		if i >= len(data) {
			return ErrInvalidSIPTradeCond
		}
		ch := data[i]
		result |= tradeCondFromChar[ch]
		i++
		if i >= len(data) || data[i] != '"' {
			return ErrInvalidSIPTradeCond
		}
		i++
	}
	*c = result
	return nil
}

// Pre-computed JSON for empty and single-condition cases
var tradeCondJSONEmpty = []byte("[]")
var tradeCondJSONSingle [64][]byte

func init() {
	for i := range 34 {
		tradeCondJSONSingle[i] = []byte{'[', '"', tradeCondToChar[i], '"', ']'}
	}
}

func (c SIPTradeCond) MarshalJSON() ([]byte, error) {
	if c == 0 {
		return tradeCondJSONEmpty, nil
	}
	// Fast path for single condition (common case) - uses tzcnt instruction
	if c&(c-1) == 0 {
		return tradeCondJSONSingle[bits.TrailingZeros64(uint64(c))], nil
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
		buf = append(buf, '"', tradeCondToChar[i], '"')
		first = false
		c &= c - 1 // clear lowest set bit
	}
	buf = append(buf, ']')
	return buf, nil
}
