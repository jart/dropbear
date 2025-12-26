package ds

type OrderStrategy int

const (
	OrderStrategyMarketable OrderStrategy = iota // maker + taker
	OrderStrategyPostOnly                        // maker
	OrderStrategyIOC                             // taker
)

func (l OrderStrategy) String() string {
	switch l {
	case OrderStrategyMarketable:
		return "marketable"
	case OrderStrategyPostOnly:
		return "postonly"
	case OrderStrategyIOC:
		return "ioc"
	default:
		panic("invalid order strategy")
	}
}
