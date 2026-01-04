package alpaca

import "fmt"

// OrderEvent is an enumeration of order event types for websocket streaming.
// See https://docs.alpaca.markets/docs/websocket-streaming
type OrderEvent uint8

const (
	// OrderEventNew is sent when an order has been routed to exchanges for execution.
	OrderEventNew OrderEvent = iota

	// OrderEventFill is sent when an order has been completely filled.
	// Includes: timestamp, price, qty, position_qty.
	OrderEventFill

	// OrderEventPartialFill is sent when a number of shares less than the total
	// remaining quantity on your order has been filled.
	// Includes: timestamp, price, qty, position_qty.
	OrderEventPartialFill

	// OrderEventCanceled is sent when your requested cancelation of an order is processed.
	// Includes: timestamp.
	OrderEventCanceled

	// OrderEventExpired is sent when an order has reached the end of its lifespan,
	// as determined by the order's time in force value.
	// Includes: timestamp.
	OrderEventExpired

	// OrderEventDoneForDay is sent when the order is done executing for the day,
	// and will not receive further updates until the next trading day.
	OrderEventDoneForDay

	// OrderEventReplaced is sent when your requested replacement of an order is processed.
	// Includes: timestamp.
	OrderEventReplaced

	// OrderEventAccepted is sent when your order has been received by Alpaca,
	// but hasn't yet been routed to the execution venue. (rare)
	OrderEventAccepted

	// OrderEventRejected is sent when your order has been rejected. (rare)
	// Includes: timestamp.
	OrderEventRejected

	// OrderEventPendingNew is sent when the order has been received by Alpaca and
	// routed to the exchanges, but has not yet been accepted for execution. (rare)
	OrderEventPendingNew

	// OrderEventStopped is sent when your order has been stopped, and a trade is
	// guaranteed for the order, usually at a stated price or better, but has not
	// yet occurred. (rare)
	OrderEventStopped

	// OrderEventPendingCancel is sent when the order is awaiting cancelation.
	// Most cancelations will occur without the order entering this state. (rare)
	OrderEventPendingCancel

	// OrderEventPendingReplace is sent when the order is awaiting replacement. (rare)
	OrderEventPendingReplace

	// OrderEventCalculated is sent when the order has been completed for the day
	// (either filled or done_for_day) but remaining settlement calculations are
	// still pending. (rare)
	OrderEventCalculated

	// OrderEventSuspended is sent when the order has been suspended and is not
	// eligible for trading. (rare)
	OrderEventSuspended

	// OrderEventReplaceRejected is sent when the order replace has been rejected. (rare)
	OrderEventReplaceRejected

	// OrderEventCancelRejected is sent when the order cancel has been rejected. (rare)
	OrderEventCancelRejected
)

func ParseOrderEvent(s string) (OrderEvent, error) {
	switch s {
	case "new":
		return OrderEventNew, nil
	case "fill":
		return OrderEventFill, nil
	case "partial_fill":
		return OrderEventPartialFill, nil
	case "canceled":
		return OrderEventCanceled, nil
	case "expired":
		return OrderEventExpired, nil
	case "done_for_day":
		return OrderEventDoneForDay, nil
	case "replaced":
		return OrderEventReplaced, nil
	case "accepted":
		return OrderEventAccepted, nil
	case "rejected":
		return OrderEventRejected, nil
	case "pending_new":
		return OrderEventPendingNew, nil
	case "stopped":
		return OrderEventStopped, nil
	case "pending_cancel":
		return OrderEventPendingCancel, nil
	case "pending_replace":
		return OrderEventPendingReplace, nil
	case "calculated":
		return OrderEventCalculated, nil
	case "suspended":
		return OrderEventSuspended, nil
	case "order_replace_rejected":
		return OrderEventReplaceRejected, nil
	case "order_cancel_rejected":
		return OrderEventCancelRejected, nil
	default:
		return 0, fmt.Errorf("unknown order event: %s", s)
	}
}

func (oe OrderEvent) String() string {
	switch oe {
	case OrderEventNew:
		return "new"
	case OrderEventFill:
		return "fill"
	case OrderEventPartialFill:
		return "partial_fill"
	case OrderEventCanceled:
		return "canceled"
	case OrderEventExpired:
		return "expired"
	case OrderEventDoneForDay:
		return "done_for_day"
	case OrderEventReplaced:
		return "replaced"
	case OrderEventAccepted:
		return "accepted"
	case OrderEventRejected:
		return "rejected"
	case OrderEventPendingNew:
		return "pending_new"
	case OrderEventStopped:
		return "stopped"
	case OrderEventPendingCancel:
		return "pending_cancel"
	case OrderEventPendingReplace:
		return "pending_replace"
	case OrderEventCalculated:
		return "calculated"
	case OrderEventSuspended:
		return "suspended"
	case OrderEventReplaceRejected:
		return "order_replace_rejected"
	case OrderEventCancelRejected:
		return "order_cancel_rejected"
	default:
		panic("unknown order event")
	}
}

func (oe OrderEvent) GoString() string {
	switch oe {
	case OrderEventNew:
		return "OrderEventNew"
	case OrderEventFill:
		return "OrderEventFill"
	case OrderEventPartialFill:
		return "OrderEventPartialFill"
	case OrderEventCanceled:
		return "OrderEventCanceled"
	case OrderEventExpired:
		return "OrderEventExpired"
	case OrderEventDoneForDay:
		return "OrderEventDoneForDay"
	case OrderEventReplaced:
		return "OrderEventReplaced"
	case OrderEventAccepted:
		return "OrderEventAccepted"
	case OrderEventRejected:
		return "OrderEventRejected"
	case OrderEventPendingNew:
		return "OrderEventPendingNew"
	case OrderEventStopped:
		return "OrderEventStopped"
	case OrderEventPendingCancel:
		return "OrderEventPendingCancel"
	case OrderEventPendingReplace:
		return "OrderEventPendingReplace"
	case OrderEventCalculated:
		return "OrderEventCalculated"
	case OrderEventSuspended:
		return "OrderEventSuspended"
	case OrderEventReplaceRejected:
		return "OrderEventReplaceRejected"
	case OrderEventCancelRejected:
		return "OrderEventCancelRejected"
	default:
		panic("unknown order event")
	}
}

func (oe OrderEvent) MarshalJSON() ([]byte, error) {
	return []byte(`"` + oe.String() + `"`), nil
}

func (oe *OrderEvent) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid order event: %s", data)
	}
	v, err := ParseOrderEvent(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*oe = v
	return nil
}
