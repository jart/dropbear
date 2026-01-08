package alpaca

import "fmt"

// SIPLULDIndicator represents the Limit Up-Limit Down indicator from the SIP feed.
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
type SIPLULDIndicator byte

const (
	LULDIndicatorNA            SIPLULDIndicator = ' ' // not applicable
	LULDIndicatorPriceBand     SIPLULDIndicator = 'A' // price band (initial publication)
	LULDIndicatorRepublished   SIPLULDIndicator = 'B' // republished price band (every 30 sec)
	LULDIndicatorNBBEntered    SIPLULDIndicator = 'C' // national best bid limit state entered
	LULDIndicatorNBBExited     SIPLULDIndicator = 'D' // national best bid limit state exited
	LULDIndicatorNBOEntered    SIPLULDIndicator = 'E' // national best offer limit state entered
	LULDIndicatorNBOExited     SIPLULDIndicator = 'F' // national best offer limit state exited
	LULDIndicatorNBBNBOEntered SIPLULDIndicator = 'G' // NBB and NBO limit state entered
	LULDIndicatorNBBNBOExited  SIPLULDIndicator = 'H' // NBB and NBO limit state exited
	LULDIndicatorNBBEntNBOExit SIPLULDIndicator = 'I' // NBB entered, NBO exited
	LULDIndicatorNBBExitNBOEnt SIPLULDIndicator = 'J' // NBB exited, NBO entered
)

// IsPriceBand returns true if this indicator represents price band information.
// When true, the UpperLimit and LowerLimit fields contain the current bands.
func (i SIPLULDIndicator) IsPriceBand() bool {
	return i == LULDIndicatorPriceBand || i == LULDIndicatorRepublished
}

// IsLimitState returns true if this indicator represents a limit state transition.
// When true, the price band fields are zero-filled.
func (i SIPLULDIndicator) IsLimitState() bool {
	return i >= LULDIndicatorNBBEntered && i <= LULDIndicatorNBBExitNBOEnt
}

// Pre-computed JSON representations
var sipLULDIndicatorJSON [256][]byte

func init() {
	for i := range sipLULDIndicatorJSON {
		sipLULDIndicatorJSON[i] = []byte{'"', byte(i), '"'}
	}
}

func (i SIPLULDIndicator) GoString() string {
	switch i {
	case LULDIndicatorNA:
		return "alpaca.LULDIndicatorNA"
	case LULDIndicatorPriceBand:
		return "alpaca.LULDIndicatorPriceBand"
	case LULDIndicatorRepublished:
		return "alpaca.LULDIndicatorRepublished"
	case LULDIndicatorNBBEntered:
		return "alpaca.LULDIndicatorNBBEntered"
	case LULDIndicatorNBBExited:
		return "alpaca.LULDIndicatorNBBExited"
	case LULDIndicatorNBOEntered:
		return "alpaca.LULDIndicatorNBOEntered"
	case LULDIndicatorNBOExited:
		return "alpaca.LULDIndicatorNBOExited"
	case LULDIndicatorNBBNBOEntered:
		return "alpaca.LULDIndicatorNBBNBOEntered"
	case LULDIndicatorNBBNBOExited:
		return "alpaca.LULDIndicatorNBBNBOExited"
	case LULDIndicatorNBBEntNBOExit:
		return "alpaca.LULDIndicatorNBBEntNBOExit"
	case LULDIndicatorNBBExitNBOEnt:
		return "alpaca.LULDIndicatorNBBExitNBOEnt"
	case 0:
		return "alpaca.SIPLULDIndicator(0)"
	default:
		return fmt.Sprintf("alpaca.SIPLULDIndicator(%q)", byte(i))
	}
}

func (i SIPLULDIndicator) String() string {
	return i.GoString()
}

func (i SIPLULDIndicator) MarshalJSON() ([]byte, error) {
	return sipLULDIndicatorJSON[i], nil
}

func (i *SIPLULDIndicator) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		*i = SIPLULDIndicator(data[1])
		return nil
	}
	if len(data) == 2 && data[0] == '"' && data[1] == '"' {
		*i = 0
		return nil
	}
	return fmt.Errorf("invalid SIPLULDIndicator: %s", data)
}
