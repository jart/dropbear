package alpaca

import "fmt"

type TimeInForce uint8

const (

	// TimeInForceDay is eligible for execution only on the day it is live.
	// By default, the order is only valid during Regular Trading Hours
	// (9:30am - 4:00pm ET). If unfilled after the closing auction, it is
	// automatically canceled. If submitted after the close, it is queued
	// and submitted the following trading day. If marked as eligible for
	// extended hours, the order can also execute during extended hours.
	TimeInForceDay TimeInForce = iota

	// TimeInForceGTC (Good Till Canceled) keeps the order active until
	// explicitly canceled. Non-marketable GTC limit orders are subject to
	// price adjustments to offset corporate actions affecting the issue.
	// DNR (Do Not Reduce) orders are not currently supported.
	TimeInForceGTC

	// TimeInForceOPG (On Open) executes only in the market opening auction.
	// Use with market/limit orders to submit MOO (Market On Open) and LOO
	// (Limit On Open) orders. Unfilled orders after open are canceled.
	// Orders submitted 9:28am-7:00pm ET are rejected. Orders after 7:00pm
	// are queued for the next day's opening auction. Routed to primary
	// exchange; execution follows exchange auction rules.
	TimeInForceOPG

	// TimeInForceCLS (On Close) executes only in the market closing auction.
	// Use with market/limit orders to submit MOC (Market On Close) and LOC
	// (Limit On Close) orders. Unfilled orders after close are canceled.
	// Orders submitted 3:50pm-7:00pm ET are rejected. Orders after 7:00pm
	// are queued for the next day's closing auction. Routed to primary
	// exchange; execution follows exchange auction rules.
	TimeInForceCLS

	// TimeInForceIOC (Immediate Or Cancel) requires all or part of the order
	// to be executed immediately. Any unfilled portion is canceled. Market
	// makers typically attempt to fill on a principal basis only, canceling
	// unfilled balance. This can result in entire order cancellation if the
	// market maker lacks inventory.
	TimeInForceIOC

	// TimeInForceFOK (Fill Or Kill) executes only if the entire order quantity
	// can be filled immediately, otherwise the entire order is canceled.
	TimeInForceFOK
)

func ParseTimeInForce(s string) (TimeInForce, error) {
	switch s {
	case "day":
		return TimeInForceDay, nil
	case "gtc":
		return TimeInForceGTC, nil
	case "opg":
		return TimeInForceOPG, nil
	case "cls":
		return TimeInForceCLS, nil
	case "ioc":
		return TimeInForceIOC, nil
	case "fok":
		return TimeInForceFOK, nil
	default:
		return 0, fmt.Errorf("unknown time in force: %s", s)
	}
}

func (tif TimeInForce) String() string {
	switch tif {
	case TimeInForceDay:
		return "day"
	case TimeInForceGTC:
		return "gtc"
	case TimeInForceOPG:
		return "opg"
	case TimeInForceCLS:
		return "cls"
	case TimeInForceIOC:
		return "ioc"
	case TimeInForceFOK:
		return "fok"
	default:
		panic("unknown time in force")
	}
}

func (tif TimeInForce) GoString() string {
	switch tif {
	case TimeInForceDay:
		return "TimeInForceDay"
	case TimeInForceGTC:
		return "TimeInForceGTC"
	case TimeInForceOPG:
		return "TimeInForceOPG"
	case TimeInForceCLS:
		return "TimeInForceCLS"
	case TimeInForceIOC:
		return "TimeInForceIOC"
	case TimeInForceFOK:
		return "TimeInForceFOK"
	default:
		panic("unknown time in force")
	}
}

func (tif TimeInForce) MarshalJSON() ([]byte, error) {
	return []byte(`"` + tif.String() + `"`), nil
}

func (tif *TimeInForce) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid time in force: %s", data)
	}
	v, err := ParseTimeInForce(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*tif = v
	return nil
}
