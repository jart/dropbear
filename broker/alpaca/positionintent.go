package alpaca

import "dropbear/ds"

type PositionIntent uint8

const (
	PositionIntentNone PositionIntent = iota
	PositionIntentBuyToOpen
	PositionIntentBuyToClose
	PositionIntentSellToOpen
	PositionIntentSellToClose
)

func ParsePositionIntent(s string) (PositionIntent, error) {
	switch s {
	case "", "none":
		return PositionIntentNone, nil
	case "buy_to_open":
		return PositionIntentBuyToOpen, nil
	case "buy_to_close":
		return PositionIntentBuyToClose, nil
	case "sell_to_open":
		return PositionIntentSellToOpen, nil
	case "sell_to_close":
		return PositionIntentSellToClose, nil
	default:
		return 0, ErrInvalidPositionIntent
	}
}

func (pi PositionIntent) Side() ds.Side {
	switch pi {
	case PositionIntentBuyToOpen, PositionIntentBuyToClose:
		return ds.SideBuy
	case PositionIntentSellToOpen, PositionIntentSellToClose:
		return ds.SideSell
	default:
		panic("position intent has no side")
	}
}

func (pi PositionIntent) IsOpen() bool {
	switch pi {
	case PositionIntentBuyToOpen, PositionIntentSellToOpen:
		return true
	case PositionIntentBuyToClose, PositionIntentSellToClose:
		return false
	default:
		panic("position intent has no open/close")
	}
}

func (pi PositionIntent) IsClose() bool {
	return !pi.IsOpen()
}

func (pi PositionIntent) String() string {
	switch pi {
	case PositionIntentNone:
		return "none"
	case PositionIntentBuyToOpen:
		return "buy_to_open"
	case PositionIntentBuyToClose:
		return "buy_to_close"
	case PositionIntentSellToOpen:
		return "sell_to_open"
	case PositionIntentSellToClose:
		return "sell_to_close"
	default:
		panic("invalid position intent")
	}
}

func (pi PositionIntent) GoString() string {
	switch pi {
	case PositionIntentNone:
		return "alpaca.PositionIntentNone"
	case PositionIntentBuyToOpen:
		return "alpaca.PositionIntentBuyToOpen"
	case PositionIntentBuyToClose:
		return "alpaca.PositionIntentBuyToClose"
	case PositionIntentSellToOpen:
		return "alpaca.PositionIntentSellToOpen"
	case PositionIntentSellToClose:
		return "alpaca.PositionIntentSellToClose"
	default:
		panic("invalid position intent")
	}
}

var positionIntentJSON = map[PositionIntent][]byte{
	PositionIntentNone:        []byte(`""`),
	PositionIntentBuyToOpen:   []byte(`"buy_to_open"`),
	PositionIntentBuyToClose:  []byte(`"buy_to_close"`),
	PositionIntentSellToOpen:  []byte(`"sell_to_open"`),
	PositionIntentSellToClose: []byte(`"sell_to_close"`),
}

func (pi PositionIntent) MarshalJSON() ([]byte, error) {
	return positionIntentJSON[pi], nil
}

func (pi *PositionIntent) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return ErrInvalidPositionIntent
	}
	v, err := ParsePositionIntent(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*pi = v
	return nil
}
