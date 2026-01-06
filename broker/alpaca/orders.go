package alpaca

import (
	"bytes"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/netty"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
}

// MarketOrder places a market order.
func (c *Client) MarketOrder(symbol string, side ds.Side, qty decimal.Decimal, tif TimeInForce) (*Order, error) {
	return c.CreateOrder(map[string]any{
		"symbol":        symbol,
		"qty":           qty,
		"side":          side,
		"type":          "market",
		"time_in_force": tif,
	})
}

// LimitOrder places a limit order.
func (c *Client) LimitOrder(symbol string, side ds.Side, quantity, limitPrice decimal.Decimal, timeInForce TimeInForce, orderAlgorithm OrderAlgorithm, orderDestination OrderDestination, extendedHours bool) (*Order, error) {
	request := map[string]any{
		"symbol":         symbol,
		"qty":            quantity,
		"side":           side,
		"type":           "limit",
		"time_in_force":  timeInForce,
		"limit_price":    limitPrice,
		"extended_hours": extendedHours,
	}
	if orderAlgorithm != OrderAlgorithmNone {
		request["advanced_instructions"] = map[string]any{
			"algorithm":   orderAlgorithm,
			"destination": orderDestination,
		}
	}
	return c.CreateOrder(request)
}

func (c *Client) CreateOrder(body map[string]any) (*Order, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling order: %w", err)
	}
	if !c.APITokenBucket.Try() {
		return nil, ds.ErrTooManyRequests
	}
	resp, err := c.Request(netty.FastHTTPClient, "POST", "/v2/orders", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		var compact bytes.Buffer
		if json.Compact(&compact, respBody) == nil {
			respBody = compact.Bytes()
		}
		if resp.StatusCode == http.StatusForbidden {
			return nil, ErrWashTrade
		}
		return nil, fmt.Errorf("order failed %d: %s", resp.StatusCode, string(respBody))
	}
	respBody, _ := io.ReadAll(resp.Body)
	var result Order
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w\nraw: %s", err, string(respBody))
	}
	return &result, nil
}

// ReplaceOrder lets you update a limit order.
func (c *Client) ReplaceOrder(orderID string, qty decimal.Decimal, limitPrice decimal.Decimal) (*Order, error) {
	if qty.IsNegative() {
		qty = qty.Neg()
	}
	jsonBody, err := json.Marshal(map[string]string{
		"qty":         qty.String(),
		"limit_price": limitPrice.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling order: %w", err)
	}
	if !c.APITokenBucket.Try() {
		return nil, ds.ErrTooManyRequests
	}
	resp, err := c.Request(netty.FastHTTPClient, "PATCH", "/v2/orders/"+orderID, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		var compact bytes.Buffer
		if json.Compact(&compact, respBody) == nil {
			respBody = compact.Bytes()
		}
		if resp.StatusCode == http.StatusForbidden {
			return nil, ds.ErrSelfTrade
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, ds.ErrOrderNotFound
		}
		if resp.StatusCode == http.StatusUnprocessableEntity {
			var errResp struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}
			if json.Unmarshal(respBody, &errResp) == nil && errResp.Code == 42210000 {
				return nil, ds.ErrOrderNotOpen
			}
		}
		return nil, fmt.Errorf("replace failed %d: %s", resp.StatusCode, string(respBody))
	}
	var result Order
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// ErrWashTrade is returned when an order would cross our own order (403 wash trade).
var ErrWashTrade = fmt.Errorf("wash trade detected")

// CancelOrder cancels an order by ID.
// Returns ErrOrderPendingReplace if order is not cancelable (422 - mid-replacement, etc.).
// Returns ErrOrderNotFound if order doesn't exist (404 - already filled/canceled).
func (c *Client) CancelOrder(orderID string) error {
	if !c.APITokenBucket.Try() {
		return ds.ErrTooManyRequests
	}
	resp, err := c.Request(netty.FastHTTPClient, "DELETE", "/v2/orders/"+orderID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return ds.ErrOrderNotFound
	case http.StatusUnprocessableEntity:
		return ds.ErrOrderPendingReplace
	default:
		respBody, _ := io.ReadAll(resp.Body)
		var compact bytes.Buffer
		if json.Compact(&compact, respBody) == nil {
			respBody = compact.Bytes()
		}
		return fmt.Errorf("cancel failed %d: %s", resp.StatusCode, string(respBody))
	}
}

// CancelAllOrders cancels all open orders.
func (c *Client) CancelAllOrders() error {
	c.APITokenBucket.Get()
	resp, err := c.Request(netty.FastHTTPClient, "DELETE", "/v2/orders", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusMultiStatus {
		respBody, _ := io.ReadAll(resp.Body)
		var compact bytes.Buffer
		if json.Compact(&compact, respBody) == nil {
			respBody = compact.Bytes()
		}
		return fmt.Errorf("cancel all failed %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// GetOrder retrieves a single order by ID.
func (c *Client) GetOrder(orderID string) (*Order, error) {
	c.APITokenBucket.Get()
	resp, err := c.Request(netty.BulkHttpClient, "GET", "/v2/orders/"+orderID, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result Order
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// GetOrders retrieves open orders.
func (c *Client) GetOrders() ([]Order, error) {
	c.APITokenBucket.Get()
	resp, err := c.Request(netty.BulkHttpClient, "GET", "/v2/orders?status=open", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result []Order
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return result, nil
}
