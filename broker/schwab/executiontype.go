package schwab

import "fmt"

type ExecutionType uint8

const (
	ExecutionTypeFill ExecutionType = iota // order was filled
)

func ParseExecutionType(s string) (ExecutionType, error) {
	switch s {
	case "FILL":
		return ExecutionTypeFill, nil
	default:
		return 0, fmt.Errorf("unknown execution type: %s", s)
	}
}

func (e ExecutionType) String() string {
	switch e {
	case ExecutionTypeFill:
		return "FILL"
	default:
		panic("unknown execution type")
	}
}

func (e ExecutionType) GoString() string {
	switch e {
	case ExecutionTypeFill:
		return "ExecutionTypeFill"
	default:
		panic("unknown execution type")
	}
}

func (e ExecutionType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + e.String() + `"`), nil
}

func (e *ExecutionType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid execution type: %s", data)
	}
	v, err := ParseExecutionType(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*e = v
	return nil
}
