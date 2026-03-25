package databento

import (
	"fmt"
)

// TriState represents unknown, true, or false.
type TriState byte

const (
	TriStateNotAvailable TriState = '~'
	TriStateNo           TriState = 'N'
	TriStateYes          TriState = 'Y'
)

func (t TriState) String() string {
	switch t {
	case TriStateNotAvailable:
		return "N/A"
	case TriStateNo:
		return "No"
	case TriStateYes:
		return "Yes"
	default:
		return fmt.Sprintf("TriState(%d)", t)
	}
}
