package schwab

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
	"fmt"
)

// NewOptionLimitOrder builds a single-leg option limit order request.
//
// Example symbol: "AAPL  260620C00200000" (6-char underlying, YYMMDD, C/P, 5+3 strike)
// instruction: InstructionBuyToOpen, InstructionSellToClose, etc.
// quantity: number of contracts
// limitPrice: limit price per contract
func NewOptionLimitOrder(symbol string, instruction Instruction, quantity int64, limitPrice decimal.Decimal) *Order {
	return &Order{
		ComplexOrderStrategyType: ComplexStrategyNone,
		OrderType:                OrderTypeLimit,
		Session:                  SessionNormal,
		Price:                    limitPrice,
		Duration:                 DurationDay,
		OrderStrategyType:        OrderStrategyTypeSingle,
		OrderLegCollection: []OrderLeg{
			{
				Instruction: instruction,
				Quantity:    decimal.FromInt64(quantity),
				Instrument: Instrument{
					Symbol: symbol,
					Type:   AssetTypeOption,
				},
			},
		},
	}
}

// CreateOrder places a new order.
// The accountHash parameter must be the hash value from GetLinkedAccounts, not the plain number.
// Returns nil on success. The Schwab API returns 201 Created with an empty body.
func (c *Client) CreateOrder(accountHash string, order *Order) error {
	c.TokenBucket.Get()
	return c.RequestJSON(netty.FastHTTPClient, "POST",
		fmt.Sprintf("/accounts/%s/orders", accountHash),
		order, nil)
}

// ReplaceOrder replaces an existing order with a new order.
// The accountHash parameter must be the hash value from GetLinkedAccounts, not the plain number.
// Returns nil on success. The Schwab API returns 201 Created with an empty body.
func (c *Client) ReplaceOrder(accountHash string, orderID int64, order *Order) error {
	c.TokenBucket.Get()
	return c.RequestJSON(netty.FastHTTPClient, "PUT",
		fmt.Sprintf("/accounts/%s/orders/%d", accountHash, orderID),
		order, nil)
}

// CancelOrder cancels an existing order.
// The accountHash parameter must be the hash value from GetLinkedAccounts, not the plain number.
// Returns nil on success. The Schwab API returns 200 OK with an empty body.
func (c *Client) CancelOrder(accountHash string, orderID int64) error {
	c.TokenBucket.Get()
	return c.RequestJSON(netty.FastHTTPClient, "DELETE",
		fmt.Sprintf("/accounts/%s/orders/%d", accountHash, orderID),
		nil, nil)
}

// GetOrder retrieves a single order by ID.
// The accountHash parameter must be the hash value from GetLinkedAccounts, not the plain number.
func (c *Client) GetOrder(accountHash string, orderID int64) (*Order, error) {
	var result Order
	err := c.RequestJSON(netty.BulkHttpClient, "GET",
		fmt.Sprintf("/accounts/%s/orders/%d", accountHash, orderID),
		nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetOrdersRequest specifies parameters for listing orders.
type GetOrdersRequest struct {
	MaxResults      int         // max orders to return (default 3000)
	FromEnteredTime clocky.Time // (required) no orders entered before this time
	ToEnteredTime   clocky.Time // (required) no orders entered after this time
	Status          OrderStatus // filter by status (zero value means no filter)
}

// GetOrders retrieves orders for an account with optional filtering.
// The accountHash parameter must be the hash value from GetLinkedAccounts, not the plain number.
// fromEnteredTime and toEnteredTime are required by the Schwab API.
func (c *Client) GetOrders(accountHash string, req *GetOrdersRequest) ([]Order, error) {
	path := fmt.Sprintf("/accounts/%s/orders", accountHash)
	if req != nil {
		sep := "?"
		if req.FromEnteredTime != 0 {
			path += sep + "fromEnteredTime=" + req.FromEnteredTime.RFC3339()
			sep = "&"
		}
		if req.ToEnteredTime != 0 {
			path += sep + "toEnteredTime=" + req.ToEnteredTime.RFC3339()
			sep = "&"
		}
		if req.MaxResults > 0 {
			path += sep + fmt.Sprintf("maxResults=%d", req.MaxResults)
			sep = "&"
		}
		if req.Status != 0 {
			path += sep + "status=" + req.Status.String()
		}
	}
	var result []Order
	err := c.RequestJSON(netty.BulkHttpClient, "GET", path, nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
