package schwab

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

// Instrument identifies a tradeable security.
type Instrument struct {
	CUSIP            string          `json:"cusip,omitempty"`
	Symbol           string          `json:"symbol"`
	Description      string          `json:"description,omitempty"`
	InstrumentID     int64           `json:"instrumentId,omitempty"`
	NetChange        decimal.Decimal `json:"netChange,omitempty"`
	Type             AssetType       `json:"assetType"`
	InstrumentType   string          `json:"type,omitempty"`             // e.g. "VANILLA", "EXCHANGE_TRADED_FUND", "SWEEP_VEHICLE"
	PutCall          string          `json:"putCall,omitempty"`          // "PUT" or "CALL" (options only)
	UnderlyingSymbol string          `json:"underlyingSymbol,omitempty"` // e.g. "$SPX" (options only)
}

// OrderLeg represents a single leg in an order.
type OrderLeg struct {
	OrderLegType   string          `json:"orderLegType,omitempty"` // e.g. "EQUITY", "OPTION"
	LegID          int64           `json:"legId,omitempty"`
	Instrument     Instrument      `json:"instrument"`
	Instruction    Instruction     `json:"instruction"`
	PositionEffect PositionEffect  `json:"positionEffect,omitempty"`
	Quantity       decimal.Decimal `json:"quantity"`
	QuantityType   string          `json:"quantityType,omitempty"` // e.g. "ALL_SHARES"
	DivCapGains    string          `json:"divCapGains,omitempty"`  // e.g. "REINVEST"
	ToSymbol       string          `json:"toSymbol,omitempty"`
}

// ExecutionLeg represents a single execution within an order activity.
type ExecutionLeg struct {
	LegID             int64           `json:"legId"`
	Price             decimal.Decimal `json:"price"`
	Quantity          decimal.Decimal `json:"quantity"`
	MismarkedQuantity decimal.Decimal `json:"mismarkedQuantity"`
	InstrumentID      int64           `json:"instrumentId"`
	Time              clocky.Time     `json:"time"`
}

// OrderActivity represents an activity that occurred on an order (e.g. a fill).
type OrderActivity struct {
	ActivityType           ActivityType    `json:"activityType"`
	ExecutionType          ExecutionType   `json:"executionType,omitempty"`
	Quantity               decimal.Decimal `json:"quantity"`
	OrderRemainingQuantity decimal.Decimal `json:"orderRemainingQuantity"`
	ExecutionLegs          []ExecutionLeg  `json:"executionLegs,omitempty"`
}

// Order represents a Schwab order, both for reading responses and for building requests.
type Order struct {
	Session                  Session           `json:"session"`
	Duration                 Duration          `json:"duration"`
	OrderType                OrderType         `json:"orderType"`
	CancelTime               clocky.Time       `json:"cancelTime,omitempty"`
	ComplexOrderStrategyType ComplexStrategy   `json:"complexOrderStrategyType,omitempty"`
	Quantity                 decimal.Decimal   `json:"quantity,omitempty"`
	FilledQuantity           decimal.Decimal   `json:"filledQuantity,omitempty"`
	RemainingQuantity        decimal.Decimal   `json:"remainingQuantity,omitempty"`
	DestinationLinkName      string            `json:"destinationLinkName,omitempty"`
	ReleaseTime              clocky.Time       `json:"releaseTime,omitempty"`
	StopPrice                decimal.Decimal   `json:"stopPrice,omitempty"`
	StopPriceLinkBasis       string            `json:"stopPriceLinkBasis,omitempty"`
	StopPriceLinkType        string            `json:"stopPriceLinkType,omitempty"`
	StopPriceOffset          decimal.Decimal   `json:"stopPriceOffset,omitempty"`
	StopType                 string            `json:"stopType,omitempty"`
	PriceLinkBasis           string            `json:"priceLinkBasis,omitempty"`
	PriceLinkType            string            `json:"priceLinkType,omitempty"`
	Price                    decimal.Decimal   `json:"price,omitempty"`
	TaxLotMethod             string            `json:"taxLotMethod,omitempty"`
	OrderLegCollection       []OrderLeg        `json:"orderLegCollection"`
	ActivationPrice          decimal.Decimal   `json:"activationPrice,omitempty"`
	SpecialInstruction       string            `json:"specialInstruction,omitempty"`
	OrderStrategyType        OrderStrategyType `json:"orderStrategyType"`
	OrderID                  int64             `json:"orderId,omitempty"`
	Cancelable               bool              `json:"cancelable,omitempty"`
	Editable                 bool              `json:"editable,omitempty"`
	Status                   OrderStatus       `json:"status,omitempty"`
	EnteredTime              clocky.Time       `json:"enteredTime,omitempty"`
	CloseTime                clocky.Time       `json:"closeTime,omitempty"`
	AccountNumber            int64             `json:"accountNumber,omitempty"`
	OrderActivityCollection  []OrderActivity   `json:"orderActivityCollection,omitempty"`
	ReplacingOrderCollection []string          `json:"replacingOrderCollection,omitempty"`
	ChildOrderStrategies     []Order           `json:"childOrderStrategies,omitempty"`
	StatusDescription        string            `json:"statusDescription,omitempty"`
}
