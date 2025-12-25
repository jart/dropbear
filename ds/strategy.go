package ds

type LimitOrderStrategy int

const (
	LimitOrderStrategyMarketable LimitOrderStrategy = iota // maker + taker
	LimitOrderStrategyPostOnly                             // maker
	LimitOrderStrategyIOC                                  // taker
)

func (l LimitOrderStrategy) String() string {
	switch l {
	case LimitOrderStrategyMarketable:
		return "marketable"
	case LimitOrderStrategyPostOnly:
		return "postonly"
	case LimitOrderStrategyIOC:
		return "ioc"
	default:
		panic("invalid limit order strategy")
	}
}
