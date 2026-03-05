package schwab

import (
	"dropbear/decimal"
	"encoding/json"
)

// OrderEvent is the top-level JSON envelope for ACCT_ACTIVITY messages.
type OrderEvent struct {
	SchwabOrderID string    `json:"SchwabOrderID"` // e.g. "1005609024296"
	AccountNumber string    `json:"AccountNumber"` // e.g. "40595135"
	BaseEvent     BaseEvent `json:"BaseEvent"`
}

// BaseEvent contains the event type and the event-specific payload.
// Only one of the event-specific fields will be populated per message.
type BaseEvent struct {
	EventType                                   string           `json:"EventType"`                                             // e.g. "OrderCreated", "OrderFillCompleted", "CancelAccepted"
	OrderCreatedEventEquityOrder                *json.RawMessage `json:"OrderCreatedEventEquityOrder,omitempty"`                // huge blob with full order details, account info, legs, quotes
	OrderAcceptedEvent                          *json.RawMessage `json:"OrderAcceptedEvent,omitempty"`                          // status="Open", has quotes at acceptance time
	ExecutionRequestedEventRoutedInfo           *RouteEvent      `json:"ExecutionRequestedEventRoutedInfo,omitempty"`           // order routed to venue (JANESTREET, DASH, CES_OPT, etc.)
	ExecutionRequestCreatedEvent                *json.RawMessage `json:"ExecutionRequestCreatedEvent,omitempty"`                // FIX ack from venue, RouteStatus="RouteFixAcknowledged"
	ExecutionRequestCompletedEvent              *json.RawMessage `json:"ExecutionRequestCompletedEvent,omitempty"`              // venue accepted/rejected, ResponseType="Accepted" or 7 (cancel ack)
	ExecutionCreatedEventExecutionInfo          *json.RawMessage `json:"ExecutionCreatedEventExecutionInfo,omitempty"`          // execution record created, has ExecutionTransType="Fill" or "UROut"
	OrderFillCompletedEventOrderLegQuantityInfo *FillEvent       `json:"OrderFillCompletedEventOrderLegQuantityInfo,omitempty"` // order filled (fully or partially)
	CancelAcceptedEvent                         *CancelEvent     `json:"CancelAcceptedEvent,omitempty"`                         // cancel request accepted, CancelRequestType="ClientCancel"
	OrderUROutCompletedEvent                    *RejectEvent     `json:"OrderUROutCompletedEvent,omitempty"`                    // order rejected/cancelled out, has ValidationDetail on reject
}

// FillEvent is Schwab's OrderFillCompleted event payload.
type FillEvent struct {
	LegID            string          `json:"LegId"`            // e.g. "1005609024296" (same as SchwabOrderID for single-leg)
	LegStatus        string          `json:"LegStatus"`        // e.g. "LegClosed"
	LegSubStatus     string          `json:"LegSubStatus"`     // e.g. "LegSubStatusFilled"
	PriceImprovement decimal.Decimal `json:"PriceImprovement"` // price improvement amount (often zero)
	QuantityInfo     struct {
		ExecutionID        string          `json:"ExecutionID"`        // e.g. "20260305-EST-ngOMS-16720352389"
		CumulativeQuantity decimal.Decimal `json:"CumulativeQuantity"` // total filled so far, e.g. 1
		LeavesQuantity     decimal.Decimal `json:"LeavesQuantity"`     // remaining unfilled, 0 when fully filled
		AveragePrice       decimal.Decimal `json:"AveragePrice"`       // average fill price, e.g. 0.25
	} `json:"QuantityInfo"`
	ExecutionInfo struct {
		ExecutionSequenceNumber       int             `json:"ExecutionSequenceNumber"`       // e.g. 1
		ExecutionID                   string          `json:"ExecutionId"`                   // e.g. "20260305-EST-ngOMS-16720352389"
		VenueExecutionID              string          `json:"VenueExecutionID"`              // e.g. "OPT100495122", "7602118590740"
		Exchange                      string          `json:"Exchange"`                      // e.g. "CBOE"
		ExecutionQuantity             decimal.Decimal `json:"ExecutionQuantity"`             // contracts filled this execution, e.g. 1
		ExecutionPrice                decimal.Decimal `json:"ExecutionPrice"`                // fill price, e.g. 0.25
		ExecutionTransType            string          `json:"ExecutionTransType"`            // "Fill" or "UROut"
		ExecutionCapacityCode         string          `json:"ExecutionCapacityCode"`         // e.g. "Agency"
		RouteName                     string          `json:"RouteName"`                     // e.g. "CES_OPT_F1_J1", "DASH_OPT_F2_J1"
		RouteSequenceNumber           int             `json:"RouteSequenceNumber"`           // e.g. 1
		ReportingCapacityCode         string          `json:"ReportingCapacityCode"`         // e.g. "RC_Agency"
		PrincipalAmount               decimal.Decimal `json:"PrincipalAmmount"`              // notional value (note: Schwab typo "Ammount")
		ActualChargedCommissionAmount decimal.Decimal `json:"ActualChargedCommissionAmount"` // e.g. 0.65
		ClientOrderID                 string          `json:"ClientOrderID"`                 // e.g. "1005609024296.1"
	} `json:"ExecutionInfo"`
	OrderInfoForTransactionPosting struct {
		LimitPrice            decimal.Decimal `json:"LimitPrice"`            // order limit price, e.g. 0.25
		OrderTypeCode         string          `json:"OrderTypeCode"`         // e.g. "Limit"
		OpenClosePositionCode string          `json:"OpenClosePositionCode"` // e.g. "PC_Open"
		BuySellCode           string          `json:"BuySellCode"`           // "Buy" or "Sell"
		Quantity              decimal.Decimal `json:"Quantity"`              // order quantity, e.g. 1
		Symbol                string          `json:"Symbol"`                // e.g. "SPXW  260305C06915000"
		SchwabSecurityID      string          `json:"SchwabSecurityID"`      // e.g. "131177361"
		AccountingRuleCode    string          `json:"AccountingRuleCode"`    // e.g. "Margin", "ShortSale"
	} `json:"OrderInfoForTransactionPosting"`
}

// RejectEvent is Schwab's OrderUROutCompleted event payload.
type RejectEvent struct {
	LegID            string          `json:"LegId"`          // e.g. "1005610854315"
	LegStatus        string          `json:"LegStatus"`      // e.g. "LegClosed"
	LegSubStatus     string          `json:"LegSubStatus"`   // e.g. "LegSubStatusCancelled"
	LeavesQuantity   decimal.Decimal `json:"LeavesQuantity"` // remaining unfilled, 0 when fully cancelled out
	CancelQuantity   decimal.Decimal `json:"CancelQuantity"` // quantity cancelled, e.g. 1
	OutCancelType    string          `json:"OutCancelType"`  // "ClientCancel" or "SystemReject"
	ValidationDetail []struct {
		SchwabOrderID        string `json:"SchwabOrderID"`        // order that was rejected
		NgOMSRuleName        string `json:"NgOMSRuleName"`        // e.g. "Regulatory_Sys_0006", "Non Standard EXP Warn"
		NgOMSRuleDescription string `json:"NgOMSRuleDescription"` // human-readable rejection reason
	} `json:"ValidationDetail"` // present on SystemReject, describes why order was rejected
}

// CancelEvent is Schwab's CancelAccepted event payload.
type CancelEvent struct {
	LifecycleSchwabOrderID   string `json:"LifecycleSchwabOrderID"` // e.g. "1005588594936"
	CancelRequestType        string `json:"CancelRequestType"`      // e.g. "ClientCancel"
	LegCancelRequestInfoList []struct {
		LegID                 string          `json:"LegID"`                 // e.g. "1005588594936"
		IntendedOrderQuantity decimal.Decimal `json:"IntendedOrderQuantity"` // original order quantity, e.g. 1
		RequestedAmount       decimal.Decimal `json:"RequestedAmount"`       // quantity requested to cancel, e.g. 1
		LegStatus             string          `json:"LegStatus"`             // e.g. "LegOpen"
		LegSubStatus          string          `json:"LegSubStatus"`          // e.g. "LegSubStatusCancelled"
	} `json:"LegCancelRequestInfoList"`
}

// RouteEvent is Schwab's ExecutionRequested event payload.
type RouteEvent struct {
	RouteSequenceNumber int    `json:"RouteSequenceNumber"` // increments per route attempt, e.g. 1, 2
	RouteRequestedBy    string `json:"RouteRequestedBy"`    // e.g. "RR_Broker"
	LegID               string `json:"LegId"`               // e.g. "1005609024296"
	RouteInfo           struct {
		RouteName           string          `json:"RouteName"`           // e.g. "JANESTREET_F2_J2", "DASH_OPT_F2_J1", "CES_OPT_F1_J1"
		RouteSequenceNumber int             `json:"RouteSequenceNumber"` // same as parent RouteSequenceNumber
		RoutedQuantity      decimal.Decimal `json:"RoutedQuantity"`      // contracts routed, e.g. 1
		RoutedPrice         decimal.Decimal `json:"RoutedPrice"`         // limit price sent to venue, e.g. 28.47
		RouteStatus         string          `json:"RouteStatus"`         // e.g. "RouteCreated"
		ClientOrderID       string          `json:"ClientOrderID"`       // e.g. "1005609024296.1"
		RouteTimeInForce    string          `json:"RouteTimeInForce"`    // e.g. "Day"
		RouteRequestedType  string          `json:"RouteRequestedType"`  // "New" or "Cancel"
	} `json:"RouteInfo"`
}

// ParseOrderEvent parses the raw JSON from an ACCT_ACTIVITY message.
func ParseOrderEvent(data json.RawMessage) *OrderEvent {
	var event OrderEvent
	if json.Unmarshal(data, &event) != nil {
		return nil
	}
	return &event
}
