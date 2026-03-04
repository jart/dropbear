package schwab

import "fmt"

type OrderType uint8

const (
	OrderTypeMarket            OrderType = iota // execute immediately at best available price
	OrderTypeLimit                              // execute at specified price or better
	OrderTypeStop                               // becomes market order when stop price is reached
	OrderTypeStopLimit                          // becomes limit order when stop price is reached
	OrderTypeTrailingStop                       // stop price trails market by specified amount
	OrderTypeCabinet                            // closing trade at a nominal price (typically $0.01)
	OrderTypeNonMarketable                      // order that cannot be immediately executed
	OrderTypeMarketOnClose                      // execute at the closing price
	OrderTypeExercise                           // exercise an options contract
	OrderTypeTrailingStopLimit                  // trailing stop that becomes a limit order
	OrderTypeNetDebit                           // multi-leg order with net debit
	OrderTypeNetCredit                          // multi-leg order with net credit
	OrderTypeNetZero                            // multi-leg order with no net cost
	OrderTypeLimitOnClose                       // limit order that executes at close
	OrderTypeUnknown                            // unknown order type
)

func ParseOrderType(s string) (OrderType, error) {
	switch s {
	case "MARKET":
		return OrderTypeMarket, nil
	case "LIMIT":
		return OrderTypeLimit, nil
	case "STOP":
		return OrderTypeStop, nil
	case "STOP_LIMIT":
		return OrderTypeStopLimit, nil
	case "TRAILING_STOP":
		return OrderTypeTrailingStop, nil
	case "CABINET":
		return OrderTypeCabinet, nil
	case "NON_MARKETABLE":
		return OrderTypeNonMarketable, nil
	case "MARKET_ON_CLOSE":
		return OrderTypeMarketOnClose, nil
	case "EXERCISE":
		return OrderTypeExercise, nil
	case "TRAILING_STOP_LIMIT":
		return OrderTypeTrailingStopLimit, nil
	case "NET_DEBIT":
		return OrderTypeNetDebit, nil
	case "NET_CREDIT":
		return OrderTypeNetCredit, nil
	case "NET_ZERO":
		return OrderTypeNetZero, nil
	case "LIMIT_ON_CLOSE":
		return OrderTypeLimitOnClose, nil
	case "UNKNOWN":
		return OrderTypeUnknown, nil
	default:
		return 0, fmt.Errorf("unknown order type: %s", s)
	}
}

func (o OrderType) String() string {
	switch o {
	case OrderTypeMarket:
		return "MARKET"
	case OrderTypeLimit:
		return "LIMIT"
	case OrderTypeStop:
		return "STOP"
	case OrderTypeStopLimit:
		return "STOP_LIMIT"
	case OrderTypeTrailingStop:
		return "TRAILING_STOP"
	case OrderTypeCabinet:
		return "CABINET"
	case OrderTypeNonMarketable:
		return "NON_MARKETABLE"
	case OrderTypeMarketOnClose:
		return "MARKET_ON_CLOSE"
	case OrderTypeExercise:
		return "EXERCISE"
	case OrderTypeTrailingStopLimit:
		return "TRAILING_STOP_LIMIT"
	case OrderTypeNetDebit:
		return "NET_DEBIT"
	case OrderTypeNetCredit:
		return "NET_CREDIT"
	case OrderTypeNetZero:
		return "NET_ZERO"
	case OrderTypeLimitOnClose:
		return "LIMIT_ON_CLOSE"
	case OrderTypeUnknown:
		return "UNKNOWN"
	default:
		panic("unknown order type")
	}
}

func (o OrderType) GoString() string {
	switch o {
	case OrderTypeMarket:
		return "OrderTypeMarket"
	case OrderTypeLimit:
		return "OrderTypeLimit"
	case OrderTypeStop:
		return "OrderTypeStop"
	case OrderTypeStopLimit:
		return "OrderTypeStopLimit"
	case OrderTypeTrailingStop:
		return "OrderTypeTrailingStop"
	case OrderTypeCabinet:
		return "OrderTypeCabinet"
	case OrderTypeNonMarketable:
		return "OrderTypeNonMarketable"
	case OrderTypeMarketOnClose:
		return "OrderTypeMarketOnClose"
	case OrderTypeExercise:
		return "OrderTypeExercise"
	case OrderTypeTrailingStopLimit:
		return "OrderTypeTrailingStopLimit"
	case OrderTypeNetDebit:
		return "OrderTypeNetDebit"
	case OrderTypeNetCredit:
		return "OrderTypeNetCredit"
	case OrderTypeNetZero:
		return "OrderTypeNetZero"
	case OrderTypeLimitOnClose:
		return "OrderTypeLimitOnClose"
	case OrderTypeUnknown:
		return "OrderTypeUnknown"
	default:
		panic("unknown order type")
	}
}

func (o OrderType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + o.String() + `"`), nil
}

func (o *OrderType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid order type: %s", data)
	}
	v, err := ParseOrderType(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*o = v
	return nil
}
