package alpaca

import (
	"fmt"
	"sync/atomic"
)

type OrderStatus int32

const (
	OrderStatusNew                OrderStatus = iota // order received by Alpaca, routed to exchanges for execution
	OrderStatusPartiallyFilled                       // order has been partially filled
	OrderStatusFilled                                // order has been filled, no further updates will occur
	OrderStatusDoneForDay                            // order is done executing for the day
	OrderStatusCanceled                              // order has been canceled by user or exchange due to time-in-force
	OrderStatusExpired                               // order has expired, no further updates will occur
	OrderStatusReplaced                              // order was replaced by another order or updated due to corporate action
	OrderStatusPendingCancel                         // order is waiting to be canceled
	OrderStatusPendingReplace                        // order is waiting to be replaced, will reject cancel requests
	OrderStatusAccepted                              // order received but not yet routed to execution venue (outside trading hours)
	OrderStatusPendingNew                            // order routed to exchanges but not yet accepted for execution (rare)
	OrderStatusAcceptedForBidding                    // order received by exchanges, being evaluated for pricing (rare)
	OrderStatusStopped                               // trade guaranteed at stated price or better, but not yet occurred (rare)
	OrderStatusRejected                              // order has been rejected by exchanges (rare)
	OrderStatusSuspended                             // order has been suspended, not eligible for trading (rare)
	OrderStatusCalculated                            // order completed but settlement calculations still pending (rare)
	OrderStatusHeld                                  // this is a child order (oto/bracket) that's waiting for its parent to fill
)

func ParseOrderStatus(s string) (OrderStatus, error) {
	switch s {
	case "new":
		return OrderStatusNew, nil
	case "partially_filled":
		return OrderStatusPartiallyFilled, nil
	case "filled":
		return OrderStatusFilled, nil
	case "done_for_day":
		return OrderStatusDoneForDay, nil
	case "canceled":
		return OrderStatusCanceled, nil
	case "expired":
		return OrderStatusExpired, nil
	case "replaced":
		return OrderStatusReplaced, nil
	case "pending_cancel":
		return OrderStatusPendingCancel, nil
	case "pending_replace":
		return OrderStatusPendingReplace, nil
	case "accepted":
		return OrderStatusAccepted, nil
	case "pending_new":
		return OrderStatusPendingNew, nil
	case "accepted_for_bidding":
		return OrderStatusAcceptedForBidding, nil
	case "stopped":
		return OrderStatusStopped, nil
	case "rejected":
		return OrderStatusRejected, nil
	case "suspended":
		return OrderStatusSuspended, nil
	case "calculated":
		return OrderStatusCalculated, nil
	case "held":
		return OrderStatusHeld, nil
	default:
		return 0, fmt.Errorf("unknown order status: %s", s)
	}
}

func (os OrderStatus) String() string {
	switch os {
	case OrderStatusNew:
		return "new"
	case OrderStatusPartiallyFilled:
		return "partially_filled"
	case OrderStatusFilled:
		return "filled"
	case OrderStatusDoneForDay:
		return "done_for_day"
	case OrderStatusCanceled:
		return "canceled"
	case OrderStatusExpired:
		return "expired"
	case OrderStatusReplaced:
		return "replaced"
	case OrderStatusPendingCancel:
		return "pending_cancel"
	case OrderStatusPendingReplace:
		return "pending_replace"
	case OrderStatusAccepted:
		return "accepted"
	case OrderStatusPendingNew:
		return "pending_new"
	case OrderStatusAcceptedForBidding:
		return "accepted_for_bidding"
	case OrderStatusStopped:
		return "stopped"
	case OrderStatusRejected:
		return "rejected"
	case OrderStatusSuspended:
		return "suspended"
	case OrderStatusCalculated:
		return "calculated"
	case OrderStatusHeld:
		return "held"
	default:
		panic("unknown order status")
	}
}

func (os OrderStatus) GoString() string {
	switch os {
	case OrderStatusNew:
		return "OrderStatusNew"
	case OrderStatusPartiallyFilled:
		return "OrderStatusPartiallyFilled"
	case OrderStatusFilled:
		return "OrderStatusFilled"
	case OrderStatusDoneForDay:
		return "OrderStatusDoneForDay"
	case OrderStatusCanceled:
		return "OrderStatusCanceled"
	case OrderStatusExpired:
		return "OrderStatusExpired"
	case OrderStatusReplaced:
		return "OrderStatusReplaced"
	case OrderStatusPendingCancel:
		return "OrderStatusPendingCancel"
	case OrderStatusPendingReplace:
		return "OrderStatusPendingReplace"
	case OrderStatusAccepted:
		return "OrderStatusAccepted"
	case OrderStatusPendingNew:
		return "OrderStatusPendingNew"
	case OrderStatusAcceptedForBidding:
		return "OrderStatusAcceptedForBidding"
	case OrderStatusStopped:
		return "OrderStatusStopped"
	case OrderStatusRejected:
		return "OrderStatusRejected"
	case OrderStatusSuspended:
		return "OrderStatusSuspended"
	case OrderStatusCalculated:
		return "OrderStatusCalculated"
	case OrderStatusHeld:
		return "OrderStatusHeld"
	default:
		panic("unknown order status")
	}
}

func (os OrderStatus) MarshalJSON() ([]byte, error) {
	return []byte(`"` + os.String() + `"`), nil
}

func (os *OrderStatus) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid order status: %s", data)
	}
	v, err := ParseOrderStatus(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*os = v
	return nil
}

// IsFinal returns true if the order is in a terminal state and will not change.
func (os OrderStatus) IsFinal() bool {
	switch os {
	case OrderStatusFilled, OrderStatusCanceled, OrderStatusExpired, OrderStatusReplaced, OrderStatusRejected:
		return true
	default:
		return false
	}
}

// IsOpen returns true if the order is still active and may be canceled.
func (os OrderStatus) IsOpen() bool {
	switch os {
	case OrderStatusNew, OrderStatusPartiallyFilled, OrderStatusAccepted, OrderStatusPendingNew, OrderStatusAcceptedForBidding, OrderStatusStopped, OrderStatusDoneForDay, OrderStatusHeld:
		return true
	default:
		return false
	}
}

// Store atomically stores v into d.
func (d *OrderStatus) Store(v OrderStatus) {
	atomic.StoreInt32((*int32)(d), int32(v))
}

// Load atomically loads and returns the value of d.
func (d *OrderStatus) Load() OrderStatus {
	return OrderStatus(atomic.LoadInt32((*int32)(d)))
}
