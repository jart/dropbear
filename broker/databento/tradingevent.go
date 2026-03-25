package databento

import (
	"fmt"
)

// TradingEvent provides further information about a status update.
type TradingEvent uint16

const (
	TradingEventNone                 TradingEvent = 0
	TradingEventNoCancel             TradingEvent = 1
	TradingEventChangeTradingSession TradingEvent = 2
	TradingEventImpliedMatchingOn    TradingEvent = 3
	TradingEventImpliedMatchingOff   TradingEvent = 4
)

func (e TradingEvent) String() string {
	switch e {
	case TradingEventNone:
		return "None"
	case TradingEventNoCancel:
		return "NoCancel"
	case TradingEventChangeTradingSession:
		return "ChangeTradingSession"
	case TradingEventImpliedMatchingOn:
		return "ImpliedMatchingOn"
	case TradingEventImpliedMatchingOff:
		return "ImpliedMatchingOff"
	default:
		return fmt.Sprintf("TradingEvent(%d)", e)
	}
}
