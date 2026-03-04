package schwab

import "fmt"

type PositionEffect uint8

const (
	PositionEffectOpening   PositionEffect = iota // opening a new position
	PositionEffectClosing                         // closing an existing position
	PositionEffectAutomatic                       // automatically determined
)

func ParsePositionEffect(s string) (PositionEffect, error) {
	switch s {
	case "OPENING":
		return PositionEffectOpening, nil
	case "CLOSING":
		return PositionEffectClosing, nil
	case "AUTOMATIC":
		return PositionEffectAutomatic, nil
	default:
		return 0, fmt.Errorf("unknown position effect: %s", s)
	}
}

func (p PositionEffect) String() string {
	switch p {
	case PositionEffectOpening:
		return "OPENING"
	case PositionEffectClosing:
		return "CLOSING"
	case PositionEffectAutomatic:
		return "AUTOMATIC"
	default:
		panic("unknown position effect")
	}
}

func (p PositionEffect) GoString() string {
	switch p {
	case PositionEffectOpening:
		return "PositionEffectOpening"
	case PositionEffectClosing:
		return "PositionEffectClosing"
	case PositionEffectAutomatic:
		return "PositionEffectAutomatic"
	default:
		panic("unknown position effect")
	}
}

func (p PositionEffect) MarshalJSON() ([]byte, error) {
	return []byte(`"` + p.String() + `"`), nil
}

func (p *PositionEffect) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid position effect: %s", data)
	}
	v, err := ParsePositionEffect(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*p = v
	return nil
}
