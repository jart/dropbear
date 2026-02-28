package databento

// Side indicates side of market for resting orders, or side of aggressor in trades
type Side byte

const (
	SideAsk  Side = 'A' // a sell order or a trade with seller as aggressor
	SideBid  Side = 'B' // a buy order or a trade with buyer as aggressor
	SideNone Side = 'N' // no side specified by original source
)

func (s Side) String() string {
	switch s {
	case SideAsk:
		return "ask"
	case SideBid:
		return "bid"
	case SideNone:
		return "none"
	default:
		return string(s)
	}
}

func (s Side) GoString() string {
	switch s {
	case SideAsk:
		return "SideAsk"
	case SideBid:
		return "SideBid"
	case SideNone:
		return "SideNone"
	default:
		return string(s)
	}
}
