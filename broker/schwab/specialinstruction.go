package schwab

import "fmt"

type SpecialInstruction uint8

const (
	SpecialInstructionNone              SpecialInstruction = iota
	SpecialInstructionAllOrNone                            // ALL_OR_NONE
	SpecialInstructionDoNotReduce                          // DO_NOT_REDUCE
	SpecialInstructionAllOrNoneDoNotReduce                 // ALL_OR_NONE_DO_NOT_REDUCE
)

func ParseSpecialInstruction(s string) (SpecialInstruction, error) {
	switch s {
	case "":
		return SpecialInstructionNone, nil
	case "ALL_OR_NONE":
		return SpecialInstructionAllOrNone, nil
	case "DO_NOT_REDUCE":
		return SpecialInstructionDoNotReduce, nil
	case "ALL_OR_NONE_DO_NOT_REDUCE":
		return SpecialInstructionAllOrNoneDoNotReduce, nil
	default:
		return 0, fmt.Errorf("unknown special instruction: %s", s)
	}
}

func (si SpecialInstruction) String() string {
	switch si {
	case SpecialInstructionNone:
		return ""
	case SpecialInstructionAllOrNone:
		return "ALL_OR_NONE"
	case SpecialInstructionDoNotReduce:
		return "DO_NOT_REDUCE"
	case SpecialInstructionAllOrNoneDoNotReduce:
		return "ALL_OR_NONE_DO_NOT_REDUCE"
	default:
		panic("unknown special instruction")
	}
}

func (si SpecialInstruction) GoString() string {
	switch si {
	case SpecialInstructionNone:
		return "SpecialInstructionNone"
	case SpecialInstructionAllOrNone:
		return "SpecialInstructionAllOrNone"
	case SpecialInstructionDoNotReduce:
		return "SpecialInstructionDoNotReduce"
	case SpecialInstructionAllOrNoneDoNotReduce:
		return "SpecialInstructionAllOrNoneDoNotReduce"
	default:
		panic("unknown special instruction")
	}
}

func (si SpecialInstruction) MarshalJSON() ([]byte, error) {
	s := si.String()
	if s == "" {
		return []byte(`""`), nil
	}
	return []byte(`"` + s + `"`), nil
}

func (si *SpecialInstruction) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid special instruction: %s", data)
	}
	v, err := ParseSpecialInstruction(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*si = v
	return nil
}
