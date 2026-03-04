package schwab

import "fmt"

type OrderStrategyType uint8

const (
	OrderStrategyTypeSingle     OrderStrategyType = iota // single order
	OrderStrategyTypeCancel                              // cancel order
	OrderStrategyTypeRecall                              // recall order
	OrderStrategyTypePair                                // pair order
	OrderStrategyTypeFlatten                             // flatten order
	OrderStrategyTypeTwoDaySwap                          // two day swap order
	OrderStrategyTypeBlastAll                            // blast all order
	OrderStrategyTypeOCO                                 // one-cancels-other: filling one cancels the other
	OrderStrategyTypeTrigger                             // one-triggers-another: filling parent submits child
)

func ParseOrderStrategyType(s string) (OrderStrategyType, error) {
	switch s {
	case "SINGLE":
		return OrderStrategyTypeSingle, nil
	case "CANCEL":
		return OrderStrategyTypeCancel, nil
	case "RECALL":
		return OrderStrategyTypeRecall, nil
	case "PAIR":
		return OrderStrategyTypePair, nil
	case "FLATTEN":
		return OrderStrategyTypeFlatten, nil
	case "TWO_DAY_SWAP":
		return OrderStrategyTypeTwoDaySwap, nil
	case "BLAST_ALL":
		return OrderStrategyTypeBlastAll, nil
	case "OCO":
		return OrderStrategyTypeOCO, nil
	case "TRIGGER":
		return OrderStrategyTypeTrigger, nil
	default:
		return 0, fmt.Errorf("unknown order strategy type: %s", s)
	}
}

func (o OrderStrategyType) String() string {
	switch o {
	case OrderStrategyTypeSingle:
		return "SINGLE"
	case OrderStrategyTypeCancel:
		return "CANCEL"
	case OrderStrategyTypeRecall:
		return "RECALL"
	case OrderStrategyTypePair:
		return "PAIR"
	case OrderStrategyTypeFlatten:
		return "FLATTEN"
	case OrderStrategyTypeTwoDaySwap:
		return "TWO_DAY_SWAP"
	case OrderStrategyTypeBlastAll:
		return "BLAST_ALL"
	case OrderStrategyTypeOCO:
		return "OCO"
	case OrderStrategyTypeTrigger:
		return "TRIGGER"
	default:
		panic("unknown order strategy type")
	}
}

func (o OrderStrategyType) GoString() string {
	switch o {
	case OrderStrategyTypeSingle:
		return "OrderStrategyTypeSingle"
	case OrderStrategyTypeCancel:
		return "OrderStrategyTypeCancel"
	case OrderStrategyTypeRecall:
		return "OrderStrategyTypeRecall"
	case OrderStrategyTypePair:
		return "OrderStrategyTypePair"
	case OrderStrategyTypeFlatten:
		return "OrderStrategyTypeFlatten"
	case OrderStrategyTypeTwoDaySwap:
		return "OrderStrategyTypeTwoDaySwap"
	case OrderStrategyTypeBlastAll:
		return "OrderStrategyTypeBlastAll"
	case OrderStrategyTypeOCO:
		return "OrderStrategyTypeOCO"
	case OrderStrategyTypeTrigger:
		return "OrderStrategyTypeTrigger"
	default:
		panic("unknown order strategy type")
	}
}

func (o OrderStrategyType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + o.String() + `"`), nil
}

func (o *OrderStrategyType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid order strategy type: %s", data)
	}
	v, err := ParseOrderStrategyType(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*o = v
	return nil
}
