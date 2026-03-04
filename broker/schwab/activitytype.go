package schwab

import "fmt"

type ActivityType uint8

const (
	ActivityTypeExecution   ActivityType = iota // order was executed (fill)
	ActivityTypeOrderAction                     // order action occurred
)

func ParseActivityType(s string) (ActivityType, error) {
	switch s {
	case "EXECUTION":
		return ActivityTypeExecution, nil
	case "ORDER_ACTION":
		return ActivityTypeOrderAction, nil
	default:
		return 0, fmt.Errorf("unknown activity type: %s", s)
	}
}

func (a ActivityType) String() string {
	switch a {
	case ActivityTypeExecution:
		return "EXECUTION"
	case ActivityTypeOrderAction:
		return "ORDER_ACTION"
	default:
		panic("unknown activity type")
	}
}

func (a ActivityType) GoString() string {
	switch a {
	case ActivityTypeExecution:
		return "ActivityTypeExecution"
	case ActivityTypeOrderAction:
		return "ActivityTypeOrderAction"
	default:
		panic("unknown activity type")
	}
}

func (a ActivityType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + a.String() + `"`), nil
}

func (a *ActivityType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid activity type: %s", data)
	}
	v, err := ParseActivityType(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*a = v
	return nil
}
