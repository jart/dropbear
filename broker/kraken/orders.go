package kraken

import (
	"dropbear/decimal"
	"dropbear/ds"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// Order represents an order from the Kraken API.
type Order struct {
	TxID        string `json:"txid"`
	OrderID     string `json:"order_id"`
	Status      string `json:"status"`
	Description struct {
		Pair      string `json:"pair"`
		Type      string `json:"type"`
		OrderType string `json:"ordertype"`
		Price     string `json:"price"`
		Price2    string `json:"price2"`
		Leverage  string `json:"leverage"`
		Order     string `json:"order"`
		Close     string `json:"close"`
	} `json:"descr"`
	Volume         string  `json:"vol"`
	VolumeExecuted string  `json:"vol_exec"`
	Cost           string  `json:"cost"`
	Fee            string  `json:"fee"`
	Price          string  `json:"price"`
	StopPrice      string  `json:"stopprice"`
	LimitPrice     string  `json:"limitprice"`
	Misc           string  `json:"misc"`
	Flags          string  `json:"oflags"`
	OpenTime       float64 `json:"opentm"`
	CloseTime      float64 `json:"closetm"`
	StartTime      float64 `json:"starttm"`
	ExpireTime     float64 `json:"expiretm"`
}

// AddOrderResult is the response from creating an order.
type AddOrderResult struct {
	Description struct {
		Order string `json:"order"`
		Close string `json:"close"`
	} `json:"descr"`
	TxID []string `json:"txid"`
}

// GenerateClientOrderID generates a new UUID suitable for use as a userref.
func GenerateClientOrderID() string {
	return uuid.New().String()
}

// MarketOrder places a market order.
func (c *Client) MarketOrder(pair string, side ds.Side, quantity decimal.Decimal, userRef string) (string, error) {
	if quantity.IsNegative() {
		return "", errors.New("quantity must be positive")
	}
	values := url.Values{}
	values.Set("pair", pair)
	values.Set("type", side.String())
	values.Set("ordertype", "market")
	values.Set("volume", quantity.String())
	if userRef != "" {
		values.Set("userref", userRef)
	}
	return c.createOrder(values)
}

// LimitOrder places a limit order.
//
// Parameters:
//   - pair: trading pair symbol, e.g. "XBTUSD" or "BTC/USD"
//   - side: SideBuy or SideSell
//   - quantity: order size (positive)
//   - limitPrice: the limit price
//   - userRef: optional user reference ID
//   - strategy: OrderStrategyPostOnly (maker), OrderStrategyIOC (taker),
//     or OrderStrategyMarketable (maker + taker)
//
// Returns:
//   - order ID if successful, otherwise empty string
//   - error if failed
func (c *Client) LimitOrder(pair string, side ds.Side, quantity, limitPrice decimal.Decimal, userRef string, strategy ds.OrderStrategy) (string, error) {
	if quantity.IsNegative() {
		return "", errors.New("quantity must be positive")
	}
	values := url.Values{}
	values.Set("pair", pair)
	values.Set("type", side.String())
	values.Set("ordertype", "limit")
	values.Set("volume", quantity.String())
	values.Set("price", limitPrice.String())
	if userRef != "" {
		values.Set("userref", userRef)
	}
	switch strategy {
	case ds.OrderStrategyPostOnly:
		values.Set("oflags", "post")
	case ds.OrderStrategyIOC:
		values.Set("timeinforce", "IOC")
	case ds.OrderStrategyMarketable:
		// default behavior, no special flags
	}
	return c.createOrder(values)
}

func (c *Client) createOrder(values url.Values) (string, error) {
	if !c.tradingLimiter.Try() {
		return "", ds.ErrTooManyRequests
	}
	resp, err := c.PostFast("/0/private/AddOrder", values)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result AddOrderResult
	if err := c.decodeResponse(resp, &result); err != nil {
		// Check for known error types
		errStr := err.Error()
		if strings.Contains(errStr, "Insufficient funds") {
			return "", ds.ErrInsufficientFunds
		}
		if strings.Contains(errStr, "Post only order") {
			return "", ds.ErrPostOnly
		}
		return "", err
	}
	if len(result.TxID) == 0 {
		return "", errors.New("no order ID returned")
	}
	return result.TxID[0], nil
}

// CancelOrder cancels a single order by ID.
func (c *Client) CancelOrder(orderID string) error {
	return c.CancelOrders([]string{orderID})
}

// CancelOrders cancels multiple orders by their IDs.
func (c *Client) CancelOrders(orderIDs []string) error {
	if !c.tradingLimiter.Try() {
		return ds.ErrTooManyRequests
	}
	for _, orderID := range orderIDs {
		values := url.Values{}
		values.Set("txid", orderID)
		resp, err := c.Post("/0/private/CancelOrder", values)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

// CancelAllOrders cancels all open orders.
func (c *Client) CancelAllOrders() error {
	c.tradingLimiter.Get()
	values := url.Values{}
	resp, err := c.Post("/0/private/CancelAll", values)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result struct {
		Count int `json:"count"`
	}
	return c.decodeResponse(resp, &result)
}

// GetOrder retrieves a single order by ID.
func (c *Client) GetOrder(orderID string) (*Order, error) {
	c.tradingLimiter.Get()
	values := url.Values{}
	values.Set("txid", orderID)
	resp, err := c.Post("/0/private/QueryOrders", values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]Order
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	order, ok := result[orderID]
	if !ok {
		return nil, ds.ErrOrderNotFound
	}
	return &order, nil
}

// ListOpenOrdersResponse is the response from listing open orders.
type ListOpenOrdersResponse struct {
	Open map[string]Order `json:"open"`
}

// ListOpenOrders returns all open orders.
func (c *Client) ListOpenOrders() (*ListOpenOrdersResponse, error) {
	c.tradingLimiter.Get()
	values := url.Values{}
	resp, err := c.Post("/0/private/OpenOrders", values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result ListOpenOrdersResponse
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// EditOrder edits an existing order.
func (c *Client) EditOrder(orderID string, quantity, price decimal.Decimal) error {
	if !c.tradingLimiter.Try() {
		return ds.ErrTooManyRequests
	}

	values := url.Values{}
	values.Set("txid", orderID)
	values.Set("volume", quantity.Abs().String())
	values.Set("price", price.String())

	resp, err := c.Post("/0/private/EditOrder", values)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		TxID       string `json:"txid"`
		OriginalID string `json:"originaltxid"`
		Volume     string `json:"volume"`
		Price      string `json:"price"`
		Status     string `json:"status"`
	}
	if err := c.decodeResponse(resp, &result); err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "Unknown order") {
			return ds.ErrOrderNotFound
		}
		if strings.Contains(errStr, "Post only order") {
			return ds.ErrPostOnly
		}
		return fmt.Errorf("edit order failed: %w", err)
	}
	return nil
}
