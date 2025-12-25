package coinbase

import (
	"bytes"
	"dropbear/decimal"
	"dropbear/ds"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
)

// Order represents an order from the Coinbase API.
type Order struct {
	OrderID               string `json:"order_id"`        // e.g. f47ac10b-58cc-4372-c567-0e02b2c3d479
	ProductID             string `json:"product_id"`      // e.g. BTC-USD
	Side                  string `json:"side"`            // e.g. buy, sell
	ClientOrderID         string `json:"client_order_id"` // e.g. f47ac10b-58cc-4372-a567-0e02b2cad479
	Status                string `json:"status"`
	CreatedTime           string `json:"created_time"`
	FilledSize            string `json:"filled_size"`
	FilledValue           string `json:"filled_value"`
	AverageFilledPrice    string `json:"average_filled_price"`
	TotalFees             string `json:"total_fees"`
	ProductType           string `json:"product_type"`
	TotalValueAfterFees   string `json:"total_value_after_fees"`
	NumberOfFills         string `json:"number_of_fills"`
	CompletionPercentage  string `json:"completion_percentage"`
	SizeInclusiveOfFees   bool   `json:"size_inclusive_of_fees"`
	PendingCancel         bool   `json:"pending_cancel"`
	SizeInQuote           bool   `json:"size_in_quote"`
	Settled               bool   `json:"settled"`
	OrderType             string `json:"order_type"`
	TimeInForce           string `json:"time_in_force"`
	RejectReason          string `json:"reject_reason"`
	RejectMessage         string `json:"reject_message"`
	CancelMessage         string `json:"cancel_message"`
	OutstandingHoldAmount string `json:"outstanding_hold_amount"`
	LastFillTime          string `json:"last_fill_time"` // e.g. 2025-12-22T00:33:53.677179Z
	RetailPortfolioID     string `json:"retail_portfolio_id"`
}

type ErrorResponse struct {
	Error                 string `json:"error"`                    // e.g. UNKNOWN_FAILURE_REASON
	Message               string `json:"message"`                  // e.g. The order configuration was invalid
	ErrorDetails          string `json:"error_details"`            // e.g. Market orders cannot be placed with empty order sizes
	PreviewFailureReason  string `json:"preview_failure_reason"`   // e.g. UNKNOWN_PREVIEW_FAILURE_REASON
	NewOrderFailureReason string `json:"new_order_failure_reason"` // e.g. UNKNOWN_FAILURE_REASON
}

// CreateOrderResponse is the response from creating an order.
type CreateOrderResponse struct {
	Success         bool          `json:"success"`
	SuccessResponse Order         `json:"success_response"`
	ErrorResponse   ErrorResponse `json:"error_response"`
}

// GenerateClientOrderID generates a new UUID suitable for use as a client_order_id.
// This should be called before subscribing to order updates to avoid race conditions.
func GenerateClientOrderID() string {
	return uuid.New().String()
}

// MarketOrder places a market order using base crypto quantity.
func (c *Client) MarketOrder(productID string, side ds.Side, quantity decimal.Decimal, clientOrderID string, costBasisMethod ds.CostBasisMethod) (string, error) {
	if clientOrderID == "" {
		clientOrderID = GenerateClientOrderID()
	}
	if quantity.IsNegative() {
		return "", errors.New("quantity must be positive")
	}
	return c.createOrder(map[string]any{
		"side":              side.ToUpper(),
		"product_id":        productID,
		"client_order_id":   clientOrderID,
		"cost_basis_method": costBasisMethod.String(),
		"sor_preference":    "SOR_DISABLED",
		"order_configuration": map[string]any{
			"market_market_ioc": map[string]string{
				"base_size": quantity.String(),
			},
		},
	})
}

// LimitOrder places a limit order.
//
// Unlike market orders, which execute immediately at the best available price,
// limit orders let you control the price at which you'll buy or sell an asset.
//
// The POST_ONLY strategy lets you place orders with low time preference, that
// will potentially take a long time to fill, but once they do, you'll pay lower
// fees as a maker, rather than as a taker. If you're buying, then you'll need to
// set a limit price below the current best asking price, otherwise you'll likely
// get a post-only error. Similarly, if you're selling, you'll need to set a limit
// price above the current best bid price. You can't cross the spread in this mode.
// The system will only fill your order if a taker takes it with a marketable order.
//
// The IOC (immediate or cancel) strategy places a limit order where taking liquidity
// is the goal. It's like a market order, except it lets you control slippage. This is
// useful if you want to trade a very large quantity. For example, if you want to sell
// a million dollars of DOGE coin without cratering the market, you could place an IOC
// limit order with a limit price slightly below the current market price. This way you
// can gobble up whatever liquidity exists for your coins, and only leave a small dent.
// Another opportunity is to arbitrage any misinformed prices between exchanges. You
// might compute a limit price beneath which you can flip on Binance for a profit.
// Unlike a fill-or-kill order, an IOC order will let you take partial fills.
//
// The MARKETABLE strategy synthesizes the two strategies above. It will execute
// immediately like an IOC order if you choose a limit price that crosses the spread.
// However, if you choose a non-marketable limit price, it will behave like a post-only
// order, resting on the order book until swooped up by a taker. You might use this if
// you have indicators that compute a fair market price. Let's say you want to buy and
// your fair price is 1% below the market. In that case, your limit order will rest on
// the books for a while. On the other hand, let's say your indicators determine fair
// price is 1% above the market. In that case, your order will execute immediately,
// at whatever improved prices all the makers are offering. So let's say BTC is at
// $90,000 and you offer to buy 1 coin for $100,000. Coinbase isn't going to pocket
// the $10,000 difference, because there's plenty of liquidity near $90,000, but
// they will charge you taker fees on the full amount.
//
// Parameters:
// - symbol: trading pair symbol, e.g. "BTC-USD"
// - quantity: positive for buy, negative for sell
// - limitPrice: bounds the execution price
// - clientOrderID: see GenerateClientOrderID() or pass empty string
// - orderStrategy: specifies execution strategy, e.g. POST_ONLY, MARKETABLE, IOC
// - costBasisMethod: controls lot selection, e.g. HIFO, LIFO, FIFO
//
// Returns:
// - order ID if successful, otherwise empty string
// - error if failed, e.g. ErrPostOnly
func (c *Client) LimitOrder(productID string, side ds.Side, quantity, limitPrice decimal.Decimal, clientOrderID string, orderStrategy ds.LimitOrderStrategy, costBasisMethod ds.CostBasisMethod) (string, error) {
	if clientOrderID == "" {
		clientOrderID = GenerateClientOrderID()
	}
	if quantity.IsNegative() {
		return "", errors.New("quantity must be positive")
	}
	var orderConfiguration map[string]any
	switch orderStrategy {
	case ds.LimitOrderStrategyMarketable:
		orderConfiguration = map[string]any{
			"limit_limit_gtc": map[string]any{
				"base_size":   quantity.String(),
				"limit_price": limitPrice.String(),
				"post_only":   false,
			},
		}
	case ds.LimitOrderStrategyPostOnly:
		orderConfiguration = map[string]any{
			"limit_limit_gtc": map[string]any{
				"base_size":   quantity.String(),
				"limit_price": limitPrice.String(),
				"post_only":   true,
			},
		}
	case ds.LimitOrderStrategyIOC:
		orderConfiguration = map[string]any{
			"sor_limit_ioc": map[string]any{
				"base_size":   quantity.String(),
				"limit_price": limitPrice.String(),
			},
		}
	default:
		panic("invalid limit order strategy")
	}
	return c.createOrder(map[string]any{
		"side":                side.ToUpper(),
		"product_id":          productID,
		"client_order_id":     clientOrderID,
		"order_configuration": orderConfiguration,
		"cost_basis_method":   costBasisMethod.String(),
		"sor_preference":      "SOR_DISABLED",
	})
}

func (c *Client) createOrder(body map[string]any) (string, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshaling order: %w", err)
	}
	resp, err := c.Request(ds.FastHTTPClient, "POST", "/api/v3/brokerage/orders", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result CreateOrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	if !result.Success {
		switch result.ErrorResponse.Error {
		case "INVALID_LIMIT_PRICE_POST_ONLY":
			return "", ds.ErrPostOnly
		case "INSUFFICIENT_FUND":
			return "", ds.ErrInsufficientFunds
		}
		switch result.ErrorResponse.PreviewFailureReason {
		case "PREVIEW_INVALID_LIMIT_PRICE_POST_ONLY":
			return "", ds.ErrPostOnly
		case "PREVIEW_INSUFFICIENT_FUND":
			return "", ds.ErrInsufficientFunds
		}
		return "", fmt.Errorf("order failed: %v: %s: %s: %s",
			body,
			result.ErrorResponse.Error,
			result.ErrorResponse.Message,
			result.ErrorResponse.ErrorDetails)
	}
	return result.SuccessResponse.OrderID, nil
}

// CancelOrders cancels orders by their IDs.
func (c *Client) CancelOrders(orderIDs []string) error {
	body := map[string]any{
		"order_ids": orderIDs,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling cancel request: %w", err)
	}
	resp, err := c.Request(ds.FastHTTPClient, "POST", "/api/v3/brokerage/orders/batch_cancel", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var compact bytes.Buffer
		if json.Compact(&compact, respBody) == nil {
			respBody = compact.Bytes()
		}
		return fmt.Errorf("cancel failed %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// CancelOrder cancels a single order by ID.
func (c *Client) CancelOrder(orderID string) error {
	return c.CancelOrders([]string{orderID})
}

// GetOrder retrieves a single order by ID.
func (c *Client) GetOrder(orderID string) (*Order, error) {
	resp, err := c.Get("/api/v3/brokerage/orders/historical/" + orderID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get order error %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Order Order `json:"order"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding order: %w", err)
	}
	return &result.Order, nil
}

// ListOrdersResponse is the response from listing orders.
type ListOrdersResponse struct {
	Orders  []Order `json:"orders"`
	HasNext bool    `json:"has_next"`
	Cursor  string  `json:"cursor"`
}

// ListOrders returns orders matching the given status filter.
// Use "OPEN" to get open orders, "PENDING" for pending, etc.
// Pass cursor from previous response to get next page, or "" for first page.
func (c *Client) ListOrders(orderStatus, cursor string) (*ListOrdersResponse, error) {
	url := "/api/v3/brokerage/orders/historical/batch"
	sep := "?"
	if orderStatus != "" {
		url += sep + "order_status=" + orderStatus
		sep = "&"
	}
	if cursor != "" {
		url += sep + "cursor=" + cursor
	}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list orders failed %d: %s", resp.StatusCode, string(respBody))
	}
	var result ListOrdersResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding orders: %w", err)
	}
	return &result, nil
}

// CancelAllOrders cancels all open orders.
func (c *Client) CancelAllOrders() error {
	var allOrderIDs []string
	cursor := ""
	for {
		orders, err := c.ListOrders("OPEN", cursor)
		if err != nil {
			return fmt.Errorf("listing open orders: %w", err)
		}
		for _, o := range orders.Orders {
			allOrderIDs = append(allOrderIDs, o.OrderID)
		}
		if !orders.HasNext {
			break
		}
		cursor = orders.Cursor
	}
	if len(allOrderIDs) == 0 {
		return nil
	}
	return c.CancelOrders(allOrderIDs)
}

// ReplaceOrder replaces an existing order with new quantity and price.
// Coinbase uses edit_order endpoint for this.
func (c *Client) ReplaceOrder(orderID string, qty decimal.Decimal, price decimal.Decimal) error {
	qty = qty.Abs()
	body := map[string]any{
		"order_id": orderID,
		"size":     qty.String(),
		"price":    price.String(),
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling edit request: %w", err)
	}
	resp, err := c.Request(ds.FastHTTPClient, "POST", "/api/v3/brokerage/orders/edit", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return ds.ErrOrderNotFound
	}
	var result struct {
		Success bool `json:"success"`
		Errors  []struct {
			EditFailureReason    string `json:"edit_failure_reason"`
			PreviewFailureReason string `json:"preview_failure_reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	if !result.Success {
		for _, e := range result.Errors {
			switch e.EditFailureReason {
			case "ORDER_NOT_FOUND":
				return ds.ErrOrderNotFound
			case "ONLY_OPEN_ORDERS_CAN_BE_EDITED":
				return ds.ErrOrderNotOpen
			case "ORDER_IS_ALREADY_BEING_REPLACED":
				return ds.ErrOrderPendingReplace
			case "CANNOT_EDIT_ORDER_PENDING_CANCEL":
				return ds.ErrOrderPendingCancel
			case "INVALID_LIMIT_PRICE_POST_ONLY":
				return ds.ErrPostOnly
			case "EDIT_REQUEST_EQUAL_TO_ORIGINAL_REQUEST":
				return nil
			case "RFQ_ORDERS_CANNOT_BE_EDITED":
				return ds.ErrRFQCannotEdit
			}
			switch e.PreviewFailureReason {
			case "PREVIEW_ORDER_NOT_FOUND":
				return ds.ErrOrderNotFound
			case "PREVIEW_INVALID_LIMIT_PRICE_POST_ONLY":
				return ds.ErrPostOnly
			}
		}
		return fmt.Errorf("edit failed: %s", string(respBody))
	}
	return nil
}
