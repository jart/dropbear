package sip

import "fmt"

// Tape represents the consolidated tape designation.
// Tape A is NYSE-listed, Tape B is NYSE Arca/regional, Tape C is NASDAQ-listed.
type Tape byte

const (
	TapeA Tape = 'A' // NYSE listed securities
	TapeB Tape = 'B' // NYSE Arca, BATS, and regional exchanges
	TapeC Tape = 'C' // NASDAQ listed securities
	TapeN Tape = 'N' // BOATS?
	TapeO Tape = 'O' // no idea
)

var tapeJSON [256][]byte

func init() {
	for i := range tapeJSON {
		tapeJSON[i] = []byte{'"', byte(i), '"'}
	}
}

func (t Tape) GoString() string {
	switch t {
	case TapeA:
		return "sip.TapeA"
	case TapeB:
		return "sip.TapeB"
	case TapeC:
		return "sip.TapeC"
	case TapeN:
		return "sip.TapeN"
	case TapeO:
		return "sip.TapeO"
	default:
		return fmt.Sprintf("sip.Tape(%q)", byte(t))
	}
}

func (t Tape) String() string {
	return t.GoString()
}

// IsCTA returns true if this tape uses CTA (Consolidated Tape Association) codes.
func (t Tape) IsCTA() bool {
	return t == TapeA || t == TapeB
}

// IsUTP returns true if this tape uses UTP (Unlisted Trading Privileges) codes.
func (t Tape) IsUTP() bool {
	return t == TapeC || t == TapeO
}

func (t Tape) MarshalJSON() ([]byte, error) {
	return tapeJSON[t], nil
}

func (t *Tape) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		*t = Tape(data[1])
		return nil
	}
	return ErrInvalidTape
}

func ParseTape(code string) Tape {
	return Tape(code[0])
}

func isUpper(c byte) bool {
	return c >= 'A' && c <= 'Z'
}
