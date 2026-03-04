package schwab

import "fmt"

type Instruction uint8

const (
	InstructionBuy         Instruction = iota // buy instrument
	InstructionSell                           // sell instrument
	InstructionBuyToCover                     // buy to cover short equity position
	InstructionSellShort                      // sell short equity
	InstructionBuyToOpen                      // buy to open option position
	InstructionBuyToClose                     // buy to close option position
	InstructionSellToOpen                     // sell to open option position (write)
	InstructionSellToClose                    // sell to close option position
)

func ParseInstruction(s string) (Instruction, error) {
	switch s {
	case "BUY":
		return InstructionBuy, nil
	case "SELL":
		return InstructionSell, nil
	case "BUY_TO_COVER":
		return InstructionBuyToCover, nil
	case "SELL_SHORT":
		return InstructionSellShort, nil
	case "BUY_TO_OPEN":
		return InstructionBuyToOpen, nil
	case "BUY_TO_CLOSE":
		return InstructionBuyToClose, nil
	case "SELL_TO_OPEN":
		return InstructionSellToOpen, nil
	case "SELL_TO_CLOSE":
		return InstructionSellToClose, nil
	default:
		return 0, fmt.Errorf("unknown instruction: %s", s)
	}
}

func (i Instruction) String() string {
	switch i {
	case InstructionBuy:
		return "BUY"
	case InstructionSell:
		return "SELL"
	case InstructionBuyToCover:
		return "BUY_TO_COVER"
	case InstructionSellShort:
		return "SELL_SHORT"
	case InstructionBuyToOpen:
		return "BUY_TO_OPEN"
	case InstructionBuyToClose:
		return "BUY_TO_CLOSE"
	case InstructionSellToOpen:
		return "SELL_TO_OPEN"
	case InstructionSellToClose:
		return "SELL_TO_CLOSE"
	default:
		panic("unknown instruction")
	}
}

func (i Instruction) GoString() string {
	switch i {
	case InstructionBuy:
		return "InstructionBuy"
	case InstructionSell:
		return "InstructionSell"
	case InstructionBuyToCover:
		return "InstructionBuyToCover"
	case InstructionSellShort:
		return "InstructionSellShort"
	case InstructionBuyToOpen:
		return "InstructionBuyToOpen"
	case InstructionBuyToClose:
		return "InstructionBuyToClose"
	case InstructionSellToOpen:
		return "InstructionSellToOpen"
	case InstructionSellToClose:
		return "InstructionSellToClose"
	default:
		panic("unknown instruction")
	}
}

func (i Instruction) MarshalJSON() ([]byte, error) {
	return []byte(`"` + i.String() + `"`), nil
}

func (i *Instruction) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid instruction: %s", data)
	}
	v, err := ParseInstruction(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*i = v
	return nil
}
