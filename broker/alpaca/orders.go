package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"dropbear/netty"
)

// Order represents an Alpaca order.
type Order struct {
	ID             string          `json:"id"`               //
	Symbol         symbol.Symbol   `json:"symbol"`           // e.g. SOLUSD
	AssetID        string          `json:"asset_id"`         //
	ClientOrderID  string          `json:"client_order_id"`  //
	Replaces       string          `json:"replaces"`         // id of the order this order replaces
	Status         OrderStatus     `json:"status"`           // e.g. filled
	Class          OrderClass      `json:"order_class"`      // e.g. simple
	Type           OrderType       `json:"type"`             // e.g. limit, market
	TimeInForce    TimeInForce     `json:"time_in_force"`    //
	Side           ds.Side         `json:"side"`             //
	Notional       decimal.Decimal `json:"notional"`         // ordered notional amount. if entered, qty will be null. can take up to 9 decimal points [rounded to 8].
	Qty            decimal.Decimal `json:"qty"`              // ordered quantity. if entered, notional will be null. can take up to 9 decimal points [rounded to 8]. required if order class is mleg.
	FilledQty      decimal.Decimal `json:"filled_qty"`       //
	FilledAvgPrice decimal.Decimal `json:"filled_avg_price"` //
	LimitPrice     decimal.Decimal `json:"limit_price"`      //
	StopPrice      decimal.Decimal `json:"stop_price"`       //
	CreatedAt      clocky.Time     `json:"created_at"`       //
	UpdatedAt      clocky.Time     `json:"updated_at"`       //
	FilledAt       clocky.Time     `json:"filled_at"`        //
	CanceledAt     clocky.Time     `json:"canceled_at"`      //
	FailedAt       clocky.Time     `json:"failed_at"`        //
	ReplacedAt     clocky.Time     `json:"replaced_at"`      //
	ExtendedHours  bool            `json:"extended_hours"`   // eligible for execution outside regular trading hours
}

type OrderRequest struct {
	Symbol               symbol.Symbol         `json:"symbol"`
	Qty                  decimal.Decimal       `json:"qty"`
	Side                 ds.Side               `json:"side"`
	Type                 OrderType             `json:"type"`
	TimeInForce          TimeInForce           `json:"time_in_force"`
	ExtendedHours        bool                  `json:"extended_hours,omitempty"` // only works with type limit and time_in_force day
	OrderClass           OrderClass            `json:"order_class,omitempty"`
	LimitPrice           decimal.Decimal       `json:"limit_price,omitempty"`
	StopPrice            decimal.Decimal       `json:"stop_price,omitempty"`
	TrailPrice           decimal.Decimal       `json:"trail_price,omitempty"`
	TrailPercent         decimal.Decimal       `json:"trail_percent,omitempty"`
	Notional             decimal.Decimal       `json:"notional,omitempty"`
	PositionIntent       PositionIntent        `json:"position_intent,omitempty"`
	ClientOrderID        string                `json:"client_order_id,omitempty"` // unique identifier for the order; automatically generated if not sent (<= 128 characters)
	AdvancedInstructions *AdvancedInstructions `json:"advanced_instructions,omitempty"`
}

// https://docs.alpaca.markets/docs/alpaca-elite-smart-router?ref=alpaca.markets
// https://alpaca.markets/learn/optimize-your-orders-with-vwap-and-twap-on-alpaca
// https://thehedgefundjournal.com/dash-financial-agency-and-algorithmic-trading/
type AdvancedInstructions struct {
	Algorithm     OrderAlgorithm   `json:"algorithm"`                // the advanced routing algorithm to use for the order
	Destination   OrderDestination `json:"destination,omitempty"`    // target exchange for order execution
	DisplayQty    decimal.Decimal  `json:"display_qty,omitempty"`    // maximum shares/contracts displayed on the exchange at any time. Must be in round lot increments
	StartTime     clocky.Time      `json:"start_time,omitempty"`     // when the algorithm is to start executing. must be within current market trading hours. defaults to now or market open. does not participate in open auction
	EndTime       clocky.Time      `json:"end_time,omitempty"`       // when the algorithm is to be done executing. must be within current market trading hours. defaults to market close. does not participate in close auction
	MaxPercentage decimal.Decimal  `json:"max_percentage,omitempty"` // maximum percentage of the ticker's period volume this order might participate in. Must be 0 < max_percentage < 1, with up to 3 decimal points precision
}

// CreateOrder places a new order.
func (c *Client) CreateOrder(body *OrderRequest) (*Order, error) {
	var result Order
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.FastHTTPClient, "POST", "/v2/orders", body, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ReplaceOrder modifies an existing order's quantity and/or limit price.
func (c *Client) ReplaceOrder(orderID string, qty decimal.Decimal, limitPrice decimal.Decimal) (*Order, error) {
	c.APITokenBucket.Get()
	var result Order
	err := c.RequestJSON(netty.FastHTTPClient, "PATCH", "/v2/orders/"+orderID, &struct {
		Qty        decimal.Decimal `json:"qty,omitempty"`
		LimitPrice decimal.Decimal `json:"limit_price,omitempty"`
	}{qty, limitPrice}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CancelOrder cancels an order by ID.
// Returns ds.ErrNotFound if order doesn't exist or was already filled/canceled.
func (c *Client) CancelOrder(orderID string) error {
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.FastHTTPClient, "DELETE", "/v2/orders/"+orderID, nil, nil)
	if err != nil {
		return err
	}
	return nil
}

// CancelAllOrders cancels all open orders.
func (c *Client) CancelAllOrders() error {
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.FastHTTPClient, "DELETE", "/v2/orders", nil, nil)
	if err != nil {
		return err
	}
	return nil
}

// GetOrder retrieves a single order by ID.
func (c *Client) GetOrder(orderID string) (*Order, error) {
	var result Order
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.BulkHttpClient, "GET", "/v2/orders/"+orderID, nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetOrders retrieves open orders.
func (c *Client) GetOrders() ([]Order, error) {
	var result []Order
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.BulkHttpClient, "GET", "/v2/orders?status=open", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
