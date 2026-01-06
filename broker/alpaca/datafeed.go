package alpaca

import "fmt"

// DataFeed specifies the source feed for market data.
type DataFeed uint8

const (
	DataFeedSIP   DataFeed = iota // all US exchanges (default)
	DataFeedIEX                   // Investors EXchange
	DataFeedOTC                   // over-the-counter exchanges
	DataFeedBoats                 // Blue Ocean ATS, overnight US trading data
)

func ParseDataFeed(s string) (DataFeed, error) {
	switch s {
	case "sip", "":
		return DataFeedSIP, nil
	case "iex":
		return DataFeedIEX, nil
	case "otc":
		return DataFeedOTC, nil
	case "boats":
		return DataFeedBoats, nil
	default:
		return 0, fmt.Errorf("unknown data feed: %s", s)
	}
}

func (f DataFeed) String() string {
	switch f {
	case DataFeedSIP:
		return "sip"
	case DataFeedIEX:
		return "iex"
	case DataFeedOTC:
		return "otc"
	case DataFeedBoats:
		return "boats"
	default:
		panic("unknown data feed")
	}
}
