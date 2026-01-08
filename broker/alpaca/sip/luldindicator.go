package sip

import "fmt"

// LULDIndicator represents the Limit Up-Limit Down indicator from the SIP feed.
//
// LULD is an SEC circuit breaker mechanism (approved April 11, 2019) that prevents
// extreme price volatility in individual securities. Price bands are calculated by
// the SIPs (CTA and Nasdaq UTP) and republished every 30 seconds.
//
// How it works:
//   - Each security has upper and lower price bands based on a reference price
//   - If the National Best Bid (NBB) or National Best Offer (NBO) touches a band,
//     the security enters a "Limit State" (indicators C-J)
//   - If the Limit State persists for 15 seconds, trading pauses for 5 minutes
//   - During the pause, the primary exchange conducts a re-opening auction
//
// Message types:
//   - Indicators A/B: Informational - just publishing the current price bands
//   - Indicators C-J: The NBBO has touched a price band (Limit State)
//
// When indicators C-J are sent, the price band fields are zero-filled since
// they represent state transitions, not new price bands.
//
// Reference: CTS Pillar Multicast Output Binary Specification, Section 6.7.6
// See also: https://www.luldplan.com/
type LULDIndicator byte

const (
	LULDIndicatorNA            LULDIndicator = ' ' // not applicable
	LULDIndicatorPriceBand     LULDIndicator = 'A' // price band (initial publication)
	LULDIndicatorRepublished   LULDIndicator = 'B' // republished price band (every 30 sec)
	LULDIndicatorNBBEntered    LULDIndicator = 'C' // national best bid limit state entered
	LULDIndicatorNBBExited     LULDIndicator = 'D' // national best bid limit state exited
	LULDIndicatorNBOEntered    LULDIndicator = 'E' // national best offer limit state entered
	LULDIndicatorNBOExited     LULDIndicator = 'F' // national best offer limit state exited
	LULDIndicatorNBBNBOEntered LULDIndicator = 'G' // NBB and NBO limit state entered
	LULDIndicatorNBBNBOExited  LULDIndicator = 'H' // NBB and NBO limit state exited
	LULDIndicatorNBBEntNBOExit LULDIndicator = 'I' // NBB entered, NBO exited
	LULDIndicatorNBBExitNBOEnt LULDIndicator = 'J' // NBB exited, NBO entered
)

// IsPriceBand returns true if this indicator represents price band information.
// When true, the UpperLimit and LowerLimit fields contain the current bands.
func (i LULDIndicator) IsPriceBand() bool {
	return i == LULDIndicatorPriceBand || i == LULDIndicatorRepublished
}

// IsLimitState returns true if this indicator represents a limit state transition.
// When true, the price band fields are zero-filled.
func (i LULDIndicator) IsLimitState() bool {
	return i >= LULDIndicatorNBBEntered && i <= LULDIndicatorNBBExitNBOEnt
}

// Pre-computed JSON representations
var luldIndicatorJSON [256][]byte

func init() {
	for i := range luldIndicatorJSON {
		luldIndicatorJSON[i] = []byte{'"', byte(i), '"'}
	}
}

func (i LULDIndicator) GoString() string {
	switch i {
	case LULDIndicatorNA:
		return "sip.LULDIndicatorNA"
	case LULDIndicatorPriceBand:
		return "sip.LULDIndicatorPriceBand"
	case LULDIndicatorRepublished:
		return "sip.LULDIndicatorRepublished"
	case LULDIndicatorNBBEntered:
		return "sip.LULDIndicatorNBBEntered"
	case LULDIndicatorNBBExited:
		return "sip.LULDIndicatorNBBExited"
	case LULDIndicatorNBOEntered:
		return "sip.LULDIndicatorNBOEntered"
	case LULDIndicatorNBOExited:
		return "sip.LULDIndicatorNBOExited"
	case LULDIndicatorNBBNBOEntered:
		return "sip.LULDIndicatorNBBNBOEntered"
	case LULDIndicatorNBBNBOExited:
		return "sip.LULDIndicatorNBBNBOExited"
	case LULDIndicatorNBBEntNBOExit:
		return "sip.LULDIndicatorNBBEntNBOExit"
	case LULDIndicatorNBBExitNBOEnt:
		return "sip.LULDIndicatorNBBExitNBOEnt"
	case 0:
		return "sip.LULDIndicator(0)"
	default:
		return fmt.Sprintf("sip.LULDIndicator(%q)", byte(i))
	}
}

func (i LULDIndicator) String() string {
	return i.GoString()
}

func (i LULDIndicator) MarshalJSON() ([]byte, error) {
	return luldIndicatorJSON[i], nil
}

func (i *LULDIndicator) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		*i = LULDIndicator(data[1])
		return nil
	}
	if len(data) == 2 && data[0] == '"' && data[1] == '"' {
		*i = 0
		return nil
	}
	return ErrInvalidLULDIndicator
}
