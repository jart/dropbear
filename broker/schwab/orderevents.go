package schwab

import (
	"dropbear/decimal"
	"encoding/json"
)

// OrderEvent is the top-level JSON envelope for ACCT_ACTIVITY messages.
type OrderEvent struct {
	SchwabOrderID OrderID         `json:"SchwabOrderID"` // e.g. "1005609024296" or 1005609024296
	AccountNumber string          `json:"AccountNumber"` // e.g. "40595135"
	BaseEvent     BaseEvent       `json:"BaseEvent"`
	RawData       json.RawMessage `json:"-"` // original JSON (not marshaled back out)
}

// BaseEvent contains the event type and the event-specific payload.
// Only one of the event-specific fields will be populated per message.
type BaseEvent struct {
	EventType                                   string                 `json:"EventType"`                                             // e.g. "OrderCreated", "OrderFillCompleted", "CancelAccepted", "ChangeCreated"
	OrderCreatedEventEquityOrder                *OrderCreatedEvent     `json:"OrderCreatedEventEquityOrder,omitempty"`                // huge blob with full order details, account info, legs, quotes
	OrderAcceptedEvent                          *json.RawMessage       `json:"OrderAcceptedEvent,omitempty"`                          // status="Open", has quotes at acceptance time
	ExecutionCreatedEventExecutionInfo          *ExecutionCreated      `json:"ExecutionCreatedEventExecutionInfo,omitempty"`          // ExecutionCreated: execution record created, has ExecutionTransType="Fill" or "UROut"
	ExecutionRequestedEventRoutedInfo           *ExecutionRequested    `json:"ExecutionRequestedEventRoutedInfo,omitempty"`           // ExecutionRequested: order routed to venue (JANESTREET, DASH, CES_OPT, etc.)
	ExecutionRequestCreatedEvent                *json.RawMessage       `json:"ExecutionRequestCreatedEvent,omitempty"`                // FIX ack from venue, RouteStatus="RouteFixAcknowledged"
	ExecutionRequestCompletedEvent              *json.RawMessage       `json:"ExecutionRequestCompletedEvent,omitempty"`              // venue accepted/rejected, ResponseType="Accepted" or 7 (cancel ack)
	OrderFillCompletedEventOrderLegQuantityInfo *FillEvent             `json:"OrderFillCompletedEventOrderLegQuantityInfo,omitempty"` // order filled (fully or partially)
	CancelAcceptedEvent                         *CancelAcknowledgement `json:"CancelAcceptedEvent,omitempty"`                         // cancel request accepted, CancelRequestType="ClientCancel"
	OrderUROutCompletedEvent                    *OrderUROutCompleted   `json:"OrderUROutCompletedEvent,omitempty"`                    // order rejected/cancelled out, has ValidationDetail on reject
	OrderExpiredEvent                           *ExpiredEvent          `json:"OrderExpiredEvent,omitempty"`                           // fok order failed to fill
	ChangeCreatedEventEquityOrder               *OrderChangeEvent      `json:"ChangeCreatedEventEquityOrder,omitempty"`               // order changed
}

// FillEvent is Schwab's OrderFillCompleted event payload.
type FillEvent struct {
	LegID                          OrderID         `json:"LegId"`            // e.g. "1005609024296" (same as SchwabOrderID for single-leg)
	LegStatus                      string          `json:"LegStatus"`        // e.g. "LegClosed"
	LegSubStatus                   string          `json:"LegSubStatus"`     // e.g. "LegSubStatusFilled"
	PriceImprovement               decimal.Decimal `json:"PriceImprovement"` // notional improvement over schwab's quoted price to cross the spread
	QuantityInfo                   QuantityInfo    `json:"QuantityInfo"`
	ExecutionInfo                  ExecutionInfo   `json:"ExecutionInfo"`
	OrderInfoForTransactionPosting OrderInfo       `json:"OrderInfoForTransactionPosting"`
}

func (fill *FillEvent) Quantity() decimal.Decimal {
	qty := fill.ExecutionInfo.ExecutionQuantity
	if fill.OrderInfoForTransactionPosting.BuySellCode == "Sell" {
		qty = qty.Neg()
	}
	return qty
}

type QuantityInfo struct {
	ExecutionID        string          `json:"ExecutionID"`        // e.g. "20260305-EST-ngOMS-16720352389"
	CumulativeQuantity decimal.Decimal `json:"CumulativeQuantity"` // total filled so far, e.g. 1
	LeavesQuantity     decimal.Decimal `json:"LeavesQuantity"`     // remaining unfilled, 0 when fully filled
	AveragePrice       decimal.Decimal `json:"AveragePrice"`       // average fill price, e.g. 0.25
}

type OrderInfo struct {
	LimitPrice            decimal.Decimal `json:"LimitPrice"`            // order limit price, e.g. 0.25
	OrderTypeCode         string          `json:"OrderTypeCode"`         // e.g. "Limit"
	OpenClosePositionCode string          `json:"OpenClosePositionCode"` // e.g. "PC_Open"
	BuySellCode           string          `json:"BuySellCode"`           // "Buy" or "Sell"
	Quantity              decimal.Decimal `json:"Quantity"`              // order quantity, e.g. 1
	Symbol                string          `json:"Symbol"`                // e.g. "SPXW  260305C06915000"
	SchwabSecurityID      string          `json:"SchwabSecurityID"`      // e.g. "131177361"
	AccountingRuleCode    string          `json:"AccountingRuleCode"`    // e.g. "Margin", "ShortSale"
	StopPrice             decimal.Decimal `json:"StopPrice"`             // stop price for stop orders
	SolicitedCode         string          `json:"SolicitedCode"`         // e.g. "Unsolicited"
	SettlementType        string          `json:"SettlementType"`        // e.g. "SettlementType_Regular"
	OrderCreatedUserID    string          `json:"OrderCreatedUserID"`    // e.g. "N9XX"
	OrderCreatedUserType  string          `json:"OrderCreatedUserType"`  // e.g. "Venue"
	ClientProductCode     string          `json:"ClientProductCode"`     // e.g. "N1"
}

type ExecutionInfo struct {
	ExecutionSequenceNumber           int                  `json:"ExecutionSequenceNumber"`           // e.g. 1
	ExecutionID                       string               `json:"ExecutionId"`                       // e.g. "20260305-EST-ngOMS-16720352389"
	VenueExecutionID                  string               `json:"VenueExecutionID"`                  // e.g. "OPT100495122", "7602118590740"
	Exchange                          string               `json:"Exchange"`                          // e.g. "CBOE"
	ExecutionQuantity                 decimal.Decimal      `json:"ExecutionQuantity"`                 // contracts filled this execution, e.g. 1
	ExecutionPrice                    decimal.Decimal      `json:"ExecutionPrice"`                    // fill price, e.g. 0.25
	ExecutionTransType                string               `json:"ExecutionTransType"`                // "Fill" or "UROut"
	ExecutionCapacityCode             string               `json:"ExecutionCapacityCode"`             // e.g. "Agency"
	RouteName                         string               `json:"RouteName"`                         // e.g. "CES_OPT_F1_J1", "DASH_OPT_F2_J1"
	CancelType                        string               `json:"CancelType,omitempty"`              // e.g. "ClientCancel"
	RouteSequenceNumber               int                  `json:"RouteSequenceNumber"`               // e.g. 1
	ReportingCapacityCode             string               `json:"ReportingCapacityCode"`             // e.g. "RC_Agency"
	PrincipalAmount                   decimal.Decimal      `json:"PrincipalAmmount"`                  // notional value (note: Schwab typo "Ammount")
	ActualChargedCommissionAmount     decimal.Decimal      `json:"ActualChargedCommissionAmount"`     // e.g. 0.65
	ActualChargedFeesCommissionAndTax FeesCommissionAndTax `json:"ActualChargedFeesCommissionAndTax"` // detailed fee breakdown
	ClientOrderID                     string               `json:"ClientOrderID"`                     // e.g. "1005609024296.1"
}

type FeesCommissionAndTax struct {
	CommissionAmount      decimal.Decimal `json:"CommissionAmount"`      // e.g. 0.65
	ORF                   decimal.Decimal `json:"ORF"`                   // Options Regulatory Fee
	IOF                   decimal.Decimal `json:"IOF"`                   // Tax on Financial Operations
	TAF                   decimal.Decimal `json:"TAF"`                   // Trading Activity Fee
	FTT                   decimal.Decimal `json:"FTT"`                   // Financial Transaction Tax
	SECFees               decimal.Decimal `json:"SECFees"`               // Securities and Exchange Commission fees (equities only, often zero for options)
	TaxWithholding1446    decimal.Decimal `json:"TaxWithholding1446"`    // tax withholding for non-resident aliens
	GoodsAndServicesTax   decimal.Decimal `json:"GoodsAndServicesTax"`   // tax on goods and services (Brazil)
	StateTaxWithholding   decimal.Decimal `json:"StateTaxWithholding"`   // state tax withholding (e.g. NY, NJ)
	FederalTaxWithholding decimal.Decimal `json:"FederalTaxWithholding"` // federal tax withholding (e.g. backup withholding)
}

func (f *FeesCommissionAndTax) Total() decimal.Decimal {
	return f.CommissionAmount.Add(f.ORF).Add(f.IOF).Add(f.TAF).Add(f.FTT).Add(f.SECFees).
		Add(f.TaxWithholding1446).Add(f.GoodsAndServicesTax).Add(f.StateTaxWithholding).Add(f.FederalTaxWithholding)
}

type OrderUROutCompleted struct {
	EventType        string             `json:"EventType"`                  // e.g. "OrderUROutCompleted"
	LegID            OrderID            `json:"LegId"`                      // e.g. "1005610854315"
	ExecutionID      string             `json:"ExecutionId"`                // e.g. "20260305-EST-ngOMS-16720352389"
	LegStatus        string             `json:"LegStatus"`                  // e.g. "LegClosed"
	LegSubStatus     string             `json:"LegSubStatus"`               // e.g. "LegSubStatusCancelled"
	LeavesQuantity   decimal.Decimal    `json:"LeavesQuantity"`             // remaining unfilled, 0 when fully cancelled out
	CancelQuantity   decimal.Decimal    `json:"CancelQuantity"`             // quantity cancelled, e.g. 1
	OutCancelType    string             `json:"OutCancelType"`              // "ClientCancel" or "SystemReject"
	RouteName        string             `json:"RouteName"`                  // e.g. "JANESTREET_F2_J2", "DASH_OPT_F2_J1", "CES_OPT_F1_J1"
	ValidationDetail []ValidationDetail `json:"ValidationDetail,omitempty"` // present on SystemReject, describes why order was rejected
}

type ValidationDetail struct {
	SchwabOrderID        OrderID `json:"SchwabOrderID"`        // order that was rejected
	NgOMSRuleName        string  `json:"NgOMSRuleName"`        // e.g. "Regulatory_Sys_0006", "Non Standard EXP Warn"
	NgOMSRuleDescription string  `json:"NgOMSRuleDescription"` // human-readable rejection reason
}

type CancelAcknowledgement struct {
	LifecycleSchwabOrderID   OrderID                `json:"LifecycleSchwabOrderID"` // e.g. "1005588594936"
	CancelRequestType        string                 `json:"CancelRequestType"`      // e.g. "ClientCancel"
	LegCancelRequestInfoList []LegCancelRequestInfo `json:"LegCancelRequestInfoList"`
}

type ExpiredEvent struct {
	EventType      string          `json:"EventType"`      // e.g. "OrderExpired"
	LegID          OrderID         `json:"LegID"`          // e.g. "1005588594936"
	ExpirationType string          `json:"ExpirationType"` // e.g. "DayOrderExpiry" (FOK)
	LeavesQuantity decimal.Decimal `json:"LeavesQuantity"` // is zero for FOK orders that failed
	CancelQuantity decimal.Decimal `json:"CancelQuantity"` // equals quantity for FOK orders that failed
	LegStatus      string          `json:"LegStatus"`      // e.g. "LegClosed" (FOK)
	LegSubStatus   string          `json:"LegSubStatus"`   // e.g. "LegSubStatusExpired" (FOK)
}

type LegCancelRequestInfo struct {
	LegID                   OrderID         `json:"LegID"`                             // e.g. "1005588594936"
	IntendedOrderQuantity   decimal.Decimal `json:"IntendedOrderQuantity"`             // original order quantity, e.g. 1
	RequestedAmount         decimal.Decimal `json:"RequestedAmount"`                   // quantity requested to cancel, e.g. 1
	LegStatus               string          `json:"LegStatus"`                         // e.g. "LegOpen"
	LegSubStatus            string          `json:"LegSubStatus"`                      // e.g. "LegSubStatusCancelled"
	ChangedNewOrderID       OrderID         `json:"ChangedNewOrderID,omitempty"`       // new order id when order was edited (not just cancelled)
	ChangedNewSchwabOrderID OrderID         `json:"ChangedNewSchwabOrderId,omitempty"` // new order id when order was edited (not just cancelled)
}

type ExecutionCreated struct {
	EventType     string        `json:"EventType"`     // e.g. "ExecutionRequested"
	LegID         OrderID       `json:"LegId"`         // e.g. "1005609024296"
	ExecutionInfo ExecutionInfo `json:"ExecutionInfo"` //
}

type ExecutionRequested struct {
	EventType           string                      `json:"EventType"`           // e.g. "ExecutionRequested"
	RouteSequenceNumber int                         `json:"RouteSequenceNumber"` // increments per route attempt, e.g. 1, 2
	RouteRequestedBy    string                      `json:"RouteRequestedBy"`    // e.g. "RR_Broker"
	LegID               OrderID                     `json:"LegId"`               // e.g. "1005609024296"
	RouteInfo           ExecutionRequestedRouteInfo `json:"RouteInfo"`
}

type ExecutionRequestedRouteInfo struct {
	RouteName           string          `json:"RouteName"`                   // e.g. "JANESTREET_F2_J2", "DASH_OPT_F2_J1", "CES_OPT_F1_J1"
	RouteSequenceNumber int             `json:"RouteSequenceNumber"`         // same as parent RouteSequenceNumber
	RoutedQuantity      decimal.Decimal `json:"RoutedQuantity"`              // contracts routed, e.g. 1
	RoutedPrice         decimal.Decimal `json:"RoutedPrice"`                 // limit price sent to venue, e.g. 28.47
	RouteInstructions   []string        `json:"RouteInstructions,omitempty"` // e.g. ["FOK"]
	RouteStatus         json.RawMessage `json:"RouteStatus"`                 // e.g. "RouteCreated", "RouteFixAcknowledged", "RouteVenueAccepted", 8 (rejected?)
	ClientOrderID       string          `json:"ClientOrderID"`               // e.g. "1005609024296.1"
	RouteTimeInForce    string          `json:"RouteTimeInForce"`            // e.g. "Day", "Fok"
	RouteRequestedType  string          `json:"RouteRequestedType"`          // "New" or "Cancel"
	Quote               QuoteInfo       `json:"Quote"`                       // quote at the time of routing
}

type OrderCreatedEvent struct {
	EventType string              `json:"EventType"` // e.g. "OrderCreated"
	Order     OrderDetailsWrapper `json:"Order"`     //
}

type OrderChangeEvent struct {
	EventType              string              `json:"EventType"`              // e.g. "ChangeCreated"
	ParentSchwabOrderID    OrderID             `json:"ParentSchwabOrderID"`    // e.g. "1005609024296"
	LifecycleSchwabOrderID OrderID             `json:"LifecycleSchwabOrderID"` // e.g. "1005610854315"
	Order                  OrderDetailsWrapper `json:"Order"`                  //
}

type OrderDetailsWrapper struct {
	SchwabOrderID OrderID      `json:"SchwabOrderID"` // e.g. "1005609024296"
	AccountNumber string       `json:"AccountNumber"` // e.g. "40595135"
	Order         OrderDetails `json:"Order"`         //
}

type OrderDetails struct {
	AccountInfo              AccountInfo              `json:"AccountInfo"`              //
	ClientChannelInfo        ClientChannelInfo        `json:"ClientChannelInfo"`        //
	LifecycleSchwabOrderID   OrderID                  `json:"LifecycleSchwabOrderID"`   // e.g. "1005610854315"
	AutoConfirm              bool                     `json:"AutoConfirm"`              // e.g. false
	SourceOMS                string                   `json:"SourceOMS"`                // e.g. "ngOMS"
	FirmID                   string                   `json:"FirmID"`                   // e.g. "CHAS"
	OrderAccount             string                   `json:"OrderAccount"`             // e.g. "TDAccount"
	AssetOrderEquityOrderLeg AssetOrderEquityOrderLeg `json:"AssetOrderEquityOrderLeg"` //
}

type AccountInfo struct {
	AccountNumber            string `json:"AccountNumber"`            // e.g. "40595135"
	AccountBranch            string `json:"AccountBranch"`            // e.g. "SJ"
	CustomerOrFirmCode       string `json:"CustomerOrFirmCode"`       // e.g. "CustomerOrFirmCode_Customer"
	OrderPlacementCustomerID string `json:"OrderPlacementCustomerID"` // e.g. "40595135"
	AccountState             string `json:"AccountState"`             // e.g. "CA"
	AccountTypeCode          string `json:"AccountTypeCode"`          // e.g. "Customer"
}

type ClientChannelInfo struct {
	ClientProductCode string `json:"ClientProductCode"` // e.g. "M1"
	EventUserID       string `json:"EventUserID"`       // e.g. "01XX"
	EventUserType     string `json:"EventUserType"`     // e.g. "Client"
}

type AssetOrderEquityOrderLeg struct {
	OrderInstruction  OrderInstruction `json:"OrderInstruction"`  //
	CommissionInfo    CommissionInfo   `json:"CommissionInfo"`    //
	AssetType         string           `json:"AssetType"`         // e.g. "MajorAssetType_EquityOption"
	TimeInForce       string           `json:"TimeInForce"`       // e.g. "Day"
	OrderTypeCode     string           `json:"OrderTypeCode"`     // e.g. "Limit"
	OrderLegs         []OrderLegInfo   `json:"OrderLegs"`         //
	OrderCapacityCode string           `json:"OrderCapacityCode"` // e.g. "OC_Agency"
	SettlementType    string           `json:"SettlementType"`    // e.g. "SettlementType_Regular"
	Rule80ACode       Rule80ACode      `json:"Rule80ACode"`       // e.g. 'I' (individual investor)
	SolicitedCode     string           `json:"SolicitedCode"`     // e.g. "Unsolicited"
	TradeTag          string           `json:"TradeTag"`          // e.g. "TA_jtunneygmailcom1744074585"
}

type OrderInstruction struct {
	HandlingInstructionCode string            `json:"HandlingInstructionCode"` // e.g. "AutomatedExecutionNoIntervention"
	ExecutionStrategy       ExecutionStrategy `json:"ExecutionStrategy"`       //
}

type ExecutionStrategy struct {
	Type                   string                  `json:"Type"` // e.g. "ES_Limit"
	LimitExecutionStrategy *LimitExecutionStrategy `json:"LimitExecutionStrategy,omitempty"`
}

type LimitExecutionStrategy struct {
	Type               string          `json:"Type"`               // e.g. "ES_Limit"
	LimitPrice         decimal.Decimal `json:"LimitPrice"`         // e.g. 0.25
	LimitPriceUnitCode string          `json:"LimitPriceUnitCode"` // e.g. "Units"
}

type CommissionInfo struct {
	EstimatedOrderQuantity    decimal.Decimal `json:"EstimatedOrderQuantity"`    // e.g. 1
	EstimatedPrincipalAmount  decimal.Decimal `json:"EstimatedPrincipalAmount"`  // e.g. 28.47
	EstimatedCommissionAmount decimal.Decimal `json:"EstimatedCommissionAmount"` // e.g. 0.65
}

type OrderLegInfo struct {
	LegID                  OrderID         `json:"LegID"`                  // e.g. "1005609024296"
	LegParentSchwabOrderID OrderID         `json:"LegParentSchwabOrderID"` // e.g. "1005609024296"
	Quantity               decimal.Decimal `json:"Quantity"`               // e.g. 1
	QuantityUnitCodeType   string          `json:"QuantityUnitCodeType"`   // e.g. "SharesOrUnits"
	LeavesQuantity         decimal.Decimal `json:"LeavesQuantity"`         // e.g. 1
	BuySellCode            string          `json:"BuySellCode"`            // "Buy" or "Sell"
	Security               SecurityInfo    `json:"Security"`               //
	QuoteOnOrderAcceptance QuoteInfo       `json:"QuoteOnOrderAcceptance"` //
}

type SecurityInfo struct {
	SchwabSecurityID     string             `json:"SchwabSecurityID"`     // e.g. "131177361"
	Symbol               string             `json:"Symbol"`               // e.g. "SPXW  260305C06915000"
	UnderlyingSymbol     string             `json:"UnderlyingSymbol"`     // e.g. "SPXW"
	MajorAssetType       string             `json:"MajorAssetType"`       // e.g. "MajorAssetType_EquityOption"
	PrimaryMarketSymbol  string             `json:"PrimaryMarketSymbol"`  // e.g. "SPXW  260305C06915000"
	ShortDescriptionText string             `json:"ShortDescriptionText"` //
	ShortName            string             `json:"ShortName"`            //
	CUSIP                string             `json:"CUSIP"`                //
	OptionSecurityInfo   OptionSecurityInfo `json:"OptionSecurityInfo"`   //
}

type OptionSecurityInfo struct {
	PutCallCode                string          `json:"PutCallCode"`                // e.g. "Put", "Call"
	UnderlyingSchwabSecurityID string          `json:"UnderlyingSchwabSecurityID"` // e.g. "131177361"
	StrikePrice                decimal.Decimal `json:"StrikePrice"`                // e.g. 6915
}

type QuoteInfo struct {
	Bid           decimal.Decimal `json:"Bid"`           //
	Ask           decimal.Decimal `json:"Ask"`           //
	BidSize       decimal.Decimal `json:"BidSize"`       //
	AskSize       decimal.Decimal `json:"AskSize"`       //
	Symbol        string          `json:"Symbol"`        // e.g. "SPXW  260305C06915000"
	QuoteTypeCode string          `json:"QuoteTypeCode"` // e.g. "Mark"
	Mid           decimal.Decimal `json:"Mid"`           //
	SchwabOrderID OrderID         `json:"SchwabOrderID"` // e.g. "1005609024296" or 1005609024296
}

// ParseOrderEvent parses the raw JSON from an ACCT_ACTIVITY message.
func ParseOrderEvent(data json.RawMessage) *OrderEvent {
	var event OrderEvent
	if json.Unmarshal(data, &event) != nil {
		return nil
	}
	event.RawData = data
	return &event
}
