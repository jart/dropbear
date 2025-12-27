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

func (os OrderState) String() string {
	switch os {
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

// Store atomically stores v into d.
func (d *OrderState) Store(v OrderState) {
	atomic.StoreInt32((*int32)(d), int32(v))
}

// Load atomically loads and returns the value of d.
func (d *OrderState) Load() OrderState {
	return OrderState(atomic.LoadInt32((*int32)(d)))
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
