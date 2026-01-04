package alpaca

import "fmt"

type OrderClass uint8

const (
	OrderClassSimple  OrderClass = iota // single order (default)
	OrderClassBracket                   // entry order with take-profit and stop-loss
	OrderClassOCO                       // one-cancels-other: two orders where filling one cancels the other
	OrderClassOTO                       // one-triggers-other: primary order triggers secondary order
	OrderClassMleg                      // multi-leg complex option strategy
)

func ParseOrderClass(s string) (OrderClass, error) {
	switch s {
	case "", "simple":
		return OrderClassSimple, nil
	case "bracket":
		return OrderClassBracket, nil
	case "oco":
		return OrderClassOCO, nil
	case "oto":
		return OrderClassOTO, nil
	case "mleg":
		return OrderClassMleg, nil
	default:
		return 0, fmt.Errorf("unknown order class: %s", s)
	}
}

func (oc OrderClass) String() string {
	switch oc {
	case OrderClassSimple:
		return "simple"
	case OrderClassBracket:
		return "bracket"
	case OrderClassOCO:
		return "oco"
	case OrderClassOTO:
		return "oto"
	case OrderClassMleg:
		return "mleg"
	default:
		panic("unknown order class")
	}
}

func (oc OrderClass) GoString() string {
	switch oc {
	case OrderClassSimple:
		return "OrderClassSimple"
	case OrderClassBracket:
		return "OrderClassBracket"
	case OrderClassOCO:
		return "OrderClassOCO"
	case OrderClassOTO:
		return "OrderClassOTO"
	case OrderClassMleg:
		return "OrderClassMleg"
	default:
		panic("unknown order class")
	}
}

func (oc OrderClass) MarshalJSON() ([]byte, error) {
	return []byte(`"` + oc.String() + `"`), nil
}

func (oc *OrderClass) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid order class: %s", data)
	}
	v, err := ParseOrderClass(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*oc = v
	return nil
}
