package ds

import "fmt"

type OrderType int

const (
	OrderTypeLimit  OrderType = iota
	OrderTypeMarket           // fills at next candle open
	OrderTypeMOC              // Market-On-Close: fills at close price
	OrderTypeMOO              // Market-On-Open: fills at open price
	OrderTypeLOC              // Limit-On-Close: limit order at close
	OrderTypeLOO              // Limit-On-Open: limit order at open
)

func (ot OrderType) String() string {
	switch ot {
	case OrderTypeLimit:
		return "limit"
	case OrderTypeMarket:
		return "market"
	case OrderTypeMOC:
		return "moc"
	case OrderTypeMOO:
		return "moo"
	case OrderTypeLOC:
		return "loc"
	case OrderTypeLOO:
		return "loo"
	default:
		panic("invalid order type")
	}
}

func ParseOrderType(s string) (OrderType, error) {
	switch s {
	case "limit", "Limit", "LIMIT":
		return OrderTypeLimit, nil
	case "market", "Market", "MARKET":
		return OrderTypeMarket, nil
	case "moc", "MOC":
		return OrderTypeMOC, nil
	case "moo", "MOO":
		return OrderTypeMOO, nil
	case "loc", "LOC":
		return OrderTypeLOC, nil
	case "loo", "LOO":
		return OrderTypeLOO, nil
	default:
		return 0, fmt.Errorf("invalid order type: %s", s)
	}
}

func MustParseOrderType(s string) OrderType {
	ot, err := ParseOrderType(s)
	if err != nil {
		panic(err)
	}
	return ot
}
