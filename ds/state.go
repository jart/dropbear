package ds

import (
	"dropbear/decimal"
	"sync/atomic"
)

type OrderState int32

const (
	OrderStateNew OrderState = iota
	OrderStateSubmitted
	OrderStatePartiallyFilled
	OrderStateUpdateSubmitted
	OrderStateCancelPending
	OrderStateFilled
	OrderStateCanceled
	OrderStateInvalid
)

// String returns a human-readable representation of the order state.
func (os *OrderState) String() string {
	switch os.Load() {
	case OrderStateNew:
		return "New"
	case OrderStateSubmitted:
		return "Submitted"
	case OrderStatePartiallyFilled:
		return "PartiallyFilled"
	case OrderStateFilled:
		return "Filled"
	case OrderStateCanceled:
		return "Canceled"
	case OrderStateInvalid:
		return "Invalid"
	case OrderStateCancelPending:
		return "CancelPending"
	case OrderStateUpdateSubmitted:
		return "UpdateSubmitted"
	default:
		panic("unknown order state")
	}
}

// GoString returns a Go-syntax representation of the order state.
func (os *OrderState) GoString() string {
	switch os.Load() {
	case OrderStateNew:
		return "OrderStateNew"
	case OrderStateSubmitted:
		return "OrderStateSubmitted"
	case OrderStatePartiallyFilled:
		return "OrderStatePartiallyFilled"
	case OrderStateFilled:
		return "OrderStateFilled"
	case OrderStateCanceled:
		return "OrderStateCanceled"
	case OrderStateInvalid:
		return "OrderStateInvalid"
	case OrderStateCancelPending:
		return "OrderStateCancelPending"
	case OrderStateUpdateSubmitted:
		return "OrderStateUpdateSubmitted"
	default:
		panic("unknown order state")
	}
}

// Store atomically stores v into d.
func (d *OrderState) Store(v OrderState) {
	atomic.StoreInt32((*int32)(d), int32(v))
}

// Load atomically loads and returns the value of d.
func (d *OrderState) Load() OrderState {
	return OrderState(atomic.LoadInt32((*int32)(d)))
}

// IsFinal returns true if the order state is a final state.
// Please note order updates may still occur after reaching a final state.
func (os OrderState) IsFinal() bool {
	switch os {
	case OrderStateFilled, OrderStateCanceled, OrderStateInvalid:
		return true
	default:
		return false
	}
}

func NewOrderStateForCoinbase(state string, cumulativeQuantity decimal.Decimal) OrderState {
	switch state {
	case "PENDING", "QUEUED":
		return OrderStateSubmitted
	case "OPEN":
		if cumulativeQuantity.IsZero() {
			return OrderStateSubmitted
		}
		return OrderStatePartiallyFilled
	case "FILLED":
		return OrderStateFilled
	case "FAILED":
		return OrderStateInvalid
	case "CANCELLED", "EXPIRED":
		return OrderStateCanceled
	case "EDIT_QUEUED":
		return OrderStateUpdateSubmitted
	case "CANCEL_QUEUED":
		return OrderStateCancelPending
	default:
		return OrderStateInvalid
	}
}
