package alpaca

import "fmt"

type PositionSide uint8

const (
	PositionSideLong PositionSide = iota
	PositionSideShort
)

func ParsePositionSide(s string) (PositionSide, error) {
	switch s {
	case "long":
		return PositionSideLong, nil
	case "short":
		return PositionSideShort, nil
	default:
		return 0, fmt.Errorf("unknown position side: %s", s)
	}
}

func (ps PositionSide) String() string {
	switch ps {
	case PositionSideLong:
		return "long"
	case PositionSideShort:
		return "short"
	default:
		panic("unknown position side")
	}
}

func (ps PositionSide) GoString() string {
	switch ps {
	case PositionSideLong:
		return "PositionSideLong"
	case PositionSideShort:
		return "PositionSideShort"
	default:
		panic("unknown position side")
	}
}

func (ps PositionSide) MarshalJSON() ([]byte, error) {
	return []byte(`"` + ps.String() + `"`), nil
}

func (ps *PositionSide) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid position side: %s", data)
	}
	v, err := ParsePositionSide(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*ps = v
	return nil
}
