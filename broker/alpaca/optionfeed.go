package alpaca

import "fmt"

// OptionFeed specifies the source feed for options market data.
type OptionFeed uint8

const (
	OptionFeedNone OptionFeed = iota
	OptionFeedOPRA
	OptionFeedIndicative
)

func ParseOptionFeed(s string) (OptionFeed, error) {
	switch s {
	case "":
		return OptionFeedNone, nil
	case "opra":
		return OptionFeedOPRA, nil
	case "indicative":
		return OptionFeedIndicative, nil
	default:
		return 0, fmt.Errorf("unknown options feed: %s", s)
	}
}

func (f OptionFeed) String() string {
	switch f {
	case OptionFeedOPRA:
		return "opra"
	case OptionFeedIndicative:
		return "indicative"
	default:
		panic("unknown options feed")
	}
}

func (e OptionFeed) MarshalJSON() ([]byte, error) {
	return []byte(`"` + e.String() + `"`), nil
}

func (e *OptionFeed) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid exchange: %s", data)
	}
	ex, err := ParseOptionFeed(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*e = ex
	return nil
}
