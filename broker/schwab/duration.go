package schwab

import "fmt"

type Duration uint8

const (
	DurationDay               Duration = iota // order is eligible for execution only on the day it is live
	DurationGoodTillCancel                    // order remains active until explicitly canceled
	DurationFillOrKill                        // entire order must fill immediately or be canceled
	DurationImmediateOrCancel                 // fill what you can immediately, cancel the rest
	DurationEndOfWeek                         // order expires at end of the current week
	DurationEndOfMonth                        // order expires at end of the current month
	DurationNextEndOfMonth                    // order expires at end of the next month
	DurationUnknown                           // unknown duration
)

func ParseDuration(s string) (Duration, error) {
	switch s {
	case "DAY":
		return DurationDay, nil
	case "GOOD_TILL_CANCEL":
		return DurationGoodTillCancel, nil
	case "FILL_OR_KILL":
		return DurationFillOrKill, nil
	case "IMMEDIATE_OR_CANCEL":
		return DurationImmediateOrCancel, nil
	case "END_OF_WEEK":
		return DurationEndOfWeek, nil
	case "END_OF_MONTH":
		return DurationEndOfMonth, nil
	case "NEXT_END_OF_MONTH":
		return DurationNextEndOfMonth, nil
	case "UNKNOWN":
		return DurationUnknown, nil
	default:
		return 0, fmt.Errorf("unknown duration: %s", s)
	}
}

func (d Duration) String() string {
	switch d {
	case DurationDay:
		return "DAY"
	case DurationGoodTillCancel:
		return "GOOD_TILL_CANCEL"
	case DurationFillOrKill:
		return "FILL_OR_KILL"
	case DurationImmediateOrCancel:
		return "IMMEDIATE_OR_CANCEL"
	case DurationEndOfWeek:
		return "END_OF_WEEK"
	case DurationEndOfMonth:
		return "END_OF_MONTH"
	case DurationNextEndOfMonth:
		return "NEXT_END_OF_MONTH"
	case DurationUnknown:
		return "UNKNOWN"
	default:
		panic("unknown duration")
	}
}

func (d Duration) GoString() string {
	switch d {
	case DurationDay:
		return "DurationDay"
	case DurationGoodTillCancel:
		return "DurationGoodTillCancel"
	case DurationFillOrKill:
		return "DurationFillOrKill"
	case DurationImmediateOrCancel:
		return "DurationImmediateOrCancel"
	case DurationEndOfWeek:
		return "DurationEndOfWeek"
	case DurationEndOfMonth:
		return "DurationEndOfMonth"
	case DurationNextEndOfMonth:
		return "DurationNextEndOfMonth"
	case DurationUnknown:
		return "DurationUnknown"
	default:
		panic("unknown duration")
	}
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.String() + `"`), nil
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid duration: %s", data)
	}
	v, err := ParseDuration(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*d = v
	return nil
}
