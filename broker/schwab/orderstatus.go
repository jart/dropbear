package schwab

import "fmt"

type OrderStatus uint8

const (
	OrderStatusAwaitingParentOrder    OrderStatus = iota // waiting for parent order to fill
	OrderStatusAwaitingCondition                         // waiting for condition to be met
	OrderStatusAwaitingStopCondition                     // waiting for stop condition to be met
	OrderStatusAwaitingManualReview                      // order is under manual review
	OrderStatusAccepted                                  // order has been accepted
	OrderStatusAwaitingUROut                             // awaiting UR out
	OrderStatusPendingActivation                         // pending activation
	OrderStatusQueued                                    // order is queued for execution
	OrderStatusWorking                                   // order is working (live in the market)
	OrderStatusRejected                                  // order was rejected
	OrderStatusPendingCancel                             // cancel request is pending
	OrderStatusCanceled                                  // order has been canceled
	OrderStatusPendingReplace                            // replace request is pending
	OrderStatusReplaced                                  // order has been replaced
	OrderStatusFilled                                    // order has been completely filled
	OrderStatusExpired                                   // order has expired
	OrderStatusNew                                       // order is new (just created)
	OrderStatusAwaitingReleaseTime                       // waiting for scheduled release time
	OrderStatusPendingAcknowledgement                    // pending acknowledgement
	OrderStatusPendingRecall                             // pending recall
	OrderStatusUnknown                                   // unknown status
)

func ParseOrderStatus(s string) (OrderStatus, error) {
	switch s {
	case "AWAITING_PARENT_ORDER":
		return OrderStatusAwaitingParentOrder, nil
	case "AWAITING_CONDITION":
		return OrderStatusAwaitingCondition, nil
	case "AWAITING_STOP_CONDITION":
		return OrderStatusAwaitingStopCondition, nil
	case "AWAITING_MANUAL_REVIEW":
		return OrderStatusAwaitingManualReview, nil
	case "ACCEPTED":
		return OrderStatusAccepted, nil
	case "AWAITING_UR_OUT":
		return OrderStatusAwaitingUROut, nil
	case "PENDING_ACTIVATION":
		return OrderStatusPendingActivation, nil
	case "QUEUED":
		return OrderStatusQueued, nil
	case "WORKING":
		return OrderStatusWorking, nil
	case "REJECTED":
		return OrderStatusRejected, nil
	case "PENDING_CANCEL":
		return OrderStatusPendingCancel, nil
	case "CANCELED":
		return OrderStatusCanceled, nil
	case "PENDING_REPLACE":
		return OrderStatusPendingReplace, nil
	case "REPLACED":
		return OrderStatusReplaced, nil
	case "FILLED":
		return OrderStatusFilled, nil
	case "EXPIRED":
		return OrderStatusExpired, nil
	case "NEW":
		return OrderStatusNew, nil
	case "AWAITING_RELEASE_TIME":
		return OrderStatusAwaitingReleaseTime, nil
	case "PENDING_ACKNOWLEDGEMENT":
		return OrderStatusPendingAcknowledgement, nil
	case "PENDING_RECALL":
		return OrderStatusPendingRecall, nil
	case "UNKNOWN":
		return OrderStatusUnknown, nil
	default:
		return 0, fmt.Errorf("unknown order status: %s", s)
	}
}

func (o OrderStatus) String() string {
	switch o {
	case OrderStatusAwaitingParentOrder:
		return "AWAITING_PARENT_ORDER"
	case OrderStatusAwaitingCondition:
		return "AWAITING_CONDITION"
	case OrderStatusAwaitingStopCondition:
		return "AWAITING_STOP_CONDITION"
	case OrderStatusAwaitingManualReview:
		return "AWAITING_MANUAL_REVIEW"
	case OrderStatusAccepted:
		return "ACCEPTED"
	case OrderStatusAwaitingUROut:
		return "AWAITING_UR_OUT"
	case OrderStatusPendingActivation:
		return "PENDING_ACTIVATION"
	case OrderStatusQueued:
		return "QUEUED"
	case OrderStatusWorking:
		return "WORKING"
	case OrderStatusRejected:
		return "REJECTED"
	case OrderStatusPendingCancel:
		return "PENDING_CANCEL"
	case OrderStatusCanceled:
		return "CANCELED"
	case OrderStatusPendingReplace:
		return "PENDING_REPLACE"
	case OrderStatusReplaced:
		return "REPLACED"
	case OrderStatusFilled:
		return "FILLED"
	case OrderStatusExpired:
		return "EXPIRED"
	case OrderStatusNew:
		return "NEW"
	case OrderStatusAwaitingReleaseTime:
		return "AWAITING_RELEASE_TIME"
	case OrderStatusPendingAcknowledgement:
		return "PENDING_ACKNOWLEDGEMENT"
	case OrderStatusPendingRecall:
		return "PENDING_RECALL"
	case OrderStatusUnknown:
		return "UNKNOWN"
	default:
		panic("unknown order status")
	}
}

func (o OrderStatus) GoString() string {
	switch o {
	case OrderStatusAwaitingParentOrder:
		return "OrderStatusAwaitingParentOrder"
	case OrderStatusAwaitingCondition:
		return "OrderStatusAwaitingCondition"
	case OrderStatusAwaitingStopCondition:
		return "OrderStatusAwaitingStopCondition"
	case OrderStatusAwaitingManualReview:
		return "OrderStatusAwaitingManualReview"
	case OrderStatusAccepted:
		return "OrderStatusAccepted"
	case OrderStatusAwaitingUROut:
		return "OrderStatusAwaitingUROut"
	case OrderStatusPendingActivation:
		return "OrderStatusPendingActivation"
	case OrderStatusQueued:
		return "OrderStatusQueued"
	case OrderStatusWorking:
		return "OrderStatusWorking"
	case OrderStatusRejected:
		return "OrderStatusRejected"
	case OrderStatusPendingCancel:
		return "OrderStatusPendingCancel"
	case OrderStatusCanceled:
		return "OrderStatusCanceled"
	case OrderStatusPendingReplace:
		return "OrderStatusPendingReplace"
	case OrderStatusReplaced:
		return "OrderStatusReplaced"
	case OrderStatusFilled:
		return "OrderStatusFilled"
	case OrderStatusExpired:
		return "OrderStatusExpired"
	case OrderStatusNew:
		return "OrderStatusNew"
	case OrderStatusAwaitingReleaseTime:
		return "OrderStatusAwaitingReleaseTime"
	case OrderStatusPendingAcknowledgement:
		return "OrderStatusPendingAcknowledgement"
	case OrderStatusPendingRecall:
		return "OrderStatusPendingRecall"
	case OrderStatusUnknown:
		return "OrderStatusUnknown"
	default:
		panic("unknown order status")
	}
}

func (o OrderStatus) MarshalJSON() ([]byte, error) {
	return []byte(`"` + o.String() + `"`), nil
}

func (o *OrderStatus) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid order status: %s", data)
	}
	v, err := ParseOrderStatus(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*o = v
	return nil
}

// IsFinal returns true if the order is in a terminal state and will not change.
func (o OrderStatus) IsFinal() bool {
	switch o {
	case OrderStatusFilled, OrderStatusCanceled, OrderStatusExpired, OrderStatusReplaced, OrderStatusRejected:
		return true
	default:
		return false
	}
}

// IsOpen returns true if the order is still active and may be canceled or replaced.
func (o OrderStatus) IsOpen() bool {
	switch o {
	case OrderStatusAccepted, OrderStatusQueued, OrderStatusWorking, OrderStatusNew,
		OrderStatusAwaitingParentOrder, OrderStatusAwaitingCondition,
		OrderStatusAwaitingStopCondition, OrderStatusPendingActivation,
		OrderStatusAwaitingReleaseTime:
		return true
	default:
		return false
	}
}
