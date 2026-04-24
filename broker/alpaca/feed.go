package alpaca

import "fmt"

// Feed specifies the source feed for equity market data.
type Feed uint8

const (
	FeedNone       Feed = iota // none
	FeedSIP                    // all US exchanges
	FeedIEX                    // Investors EXchange
	FeedDelayedSIP             // delayed SIP data (15 minute delay)
	FeedBOATS                  // Blue Ocean ATS, overnight US trading data
	FeedOvernight              // Blue Ocean ATS, overnight US trading data (15 minute delay)
	FeedOTC                    // over-the-counter exchanges
)

func ParseFeed(s string) (Feed, error) {
	switch s {
	case "":
		return FeedNone, nil
	case "sip":
		return FeedSIP, nil
	case "iex":
		return FeedIEX, nil
	case "delayed_sip":
		return FeedDelayedSIP, nil
	case "boats":
		return FeedBOATS, nil
	case "overnight":
		return FeedOvernight, nil
	case "otc":
		return FeedOTC, nil
	default:
		return 0, fmt.Errorf("unknown data feed: %s", s)
	}
}

func (f Feed) String() string {
	switch f {
	case FeedSIP:
		return "sip"
	case FeedIEX:
		return "iex"
	case FeedDelayedSIP:
		return "delayed_sip"
	case FeedBOATS:
		return "boats"
	case FeedOvernight:
		return "overnight"
	case FeedOTC:
		return "otc"
	default:
		panic("unknown data feed")
	}
}

func (e Feed) MarshalJSON() ([]byte, error) {
	return []byte(`"` + e.String() + `"`), nil
}

func (e *Feed) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid exchange: %s", data)
	}
	ex, err := ParseFeed(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*e = ex
	return nil
}
