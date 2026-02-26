package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/netty"
	"fmt"
)

// Order represents an Alpaca order.
type Order struct {
	ID             string          `json:"id"`               //
	Symbol         string          `json:"symbol"`           // e.g. SOLUSD
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
	Legs           []Order         `json:"legs,omitempty"`   // nested leg orders for multi-leg orders
}

// OrderLeg represents a single leg in a multi-leg order.
type OrderLeg struct {
	Symbol         string          `json:"symbol"`
	RatioQty       decimal.Decimal `json:"ratio_qty"`
	Side           ds.Side         `json:"side"`
	PositionIntent PositionIntent  `json:"position_intent,omitempty"`
}

type OrderRequest struct {
	Symbol               string                `json:"symbol,omitempty"`
	Qty                  decimal.Decimal       `json:"qty"`
	Side                 ds.Side               `json:"side,omitempty"`
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
	TakeProfit           *TakeProfit           `json:"take_profit,omitempty"`
	StopLoss             *StopLoss             `json:"stop_loss,omitempty"`
	Legs                 []OrderLeg            `json:"legs,omitempty"`
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

type TakeProfit struct {
	LimitPrice decimal.Decimal `json:"limit_price"`
}

type StopLoss struct {
	StopPrice  decimal.Decimal `json:"stop_price"`
	LimitPrice decimal.Decimal `json:"limit_price,omitempty"`
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

// GetOrdersRequest specifies parameters for listing orders.
type GetOrdersRequest struct {
	Status    string      // open, closed, or all (default: open)
	Limit     int         // max orders to return (default 50, max 500)
	After     clocky.Time // orders created after this time
	Until     clocky.Time // orders created before this time
	Direction string      // asc or desc (default: desc)
	Nested    bool        // include multi-leg order legs
	Symbols   []string    // filter by symbols
}

// GetOrders retrieves orders with optional filtering.
// Pass nil for default behavior (open orders only).
// https://docs.alpaca.markets/reference/getallorders
func (c *Client) GetOrders(req *GetOrdersRequest) ([]Order, error) {
	path := "/v2/orders"
	if req == nil {
		path += "?status=open"
	} else {
		q := make([]string, 0, 8)
		if req.Status != "" {
			q = append(q, "status="+req.Status)
		} else {
			q = append(q, "status=open")
		}
		if req.Limit > 0 {
			q = append(q, fmt.Sprintf("limit=%d", req.Limit))
		}
		if req.After != 0 {
			q = append(q, "after="+req.After.RFC3339())
		}
		if req.Until != 0 {
			q = append(q, "until="+req.Until.RFC3339())
		}
		if req.Direction != "" {
			q = append(q, "direction="+req.Direction)
		}
		if req.Nested {
			q = append(q, "nested=true")
		}
		if len(req.Symbols) > 0 {
			var syms string
			for i, s := range req.Symbols {
				if i > 0 {
					syms += ","
				}
				syms += s
			}
			q = append(q, "symbols="+syms)
		}
		if len(q) > 0 {
			path += "?"
			for i, p := range q {
				if i > 0 {
					path += "&"
				}
				path += p
			}
		}
	}
	var result []Order
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.BulkHttpClient, "GET", path, nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
