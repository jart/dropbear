package alpaca

import "fmt"

// SIPTape represents the consolidated tape designation.
// Tape A is NYSE-listed, Tape B is NYSE Arca/regional, Tape C is NASDAQ-listed.
type SIPTape byte

const (
	SIPTapeUnknown SIPTape = 0
	SIPTapeA       SIPTape = 'A' // NYSE listed securities
	SIPTapeB       SIPTape = 'B' // NYSE Arca, BATS, and regional exchanges
	SIPTapeC       SIPTape = 'C' // NASDAQ listed securities
)

// Pre-computed JSON representations
var sipTapeJSON [256][]byte

// Pre-computed boolean LUTs
var sipTapeIsCTA [256]bool
var sipTapeIsUTP [256]bool

func init() {
	for i := range sipTapeJSON {
		sipTapeJSON[i] = []byte{'"', byte(i), '"'}
	}
	sipTapeIsCTA[SIPTapeA] = true
	sipTapeIsCTA[SIPTapeB] = true
	sipTapeIsUTP[SIPTapeC] = true
}

func (t SIPTape) GoString() string {
	switch t {
	case SIPTapeA:
		return "SIPTapeA"
	case SIPTapeB:
		return "SIPTapeB"
	case SIPTapeC:
		return "SIPTapeC"
	default:
		return fmt.Sprintf("SIPTape(%c)", byte(t))
	}
}

func (t SIPTape) String() string {
	return t.GoString()
}

func (t SIPTape) Code() string {
	return string(byte(t))
}

// IsCTA returns true if this tape uses CTA (Consolidated Tape Association) codes.
// Tapes A and B use CTA; Tape C uses UTP (NASDAQ).
func (t SIPTape) IsCTA() bool {
	return sipTapeIsCTA[t]
}

// IsUTP returns true if this tape uses UTP (Unlisted Trading Privileges) codes.
// Tape C (NASDAQ) uses UTP codes.
func (t SIPTape) IsUTP() bool {
	return sipTapeIsUTP[t]
}

func (t SIPTape) MarshalJSON() ([]byte, error) {
	return sipTapeJSON[t], nil
}

func (t *SIPTape) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		*t = SIPTape(data[1])
		return nil
	}
	if len(data) == 2 && data[0] == '"' && data[1] == '"' {
		*t = SIPTapeUnknown
		return nil
	}
	return fmt.Errorf("invalid SIPTape: %s", data)
}

func ParseSIPTape(code string) SIPTape {
	if len(code) == 0 {
		return SIPTapeUnknown
	}
	return SIPTape(code[0])
}
