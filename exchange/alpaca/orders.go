package alpaca

import (
	"bytes"
	"dropbear/decimal"
	"dropbear/ds"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Order represents an Alpaca order.
type Order struct {
	ID             string `json:"id"`
	Symbol         string `json:"symbol"` // e.g. SOLUSD
	AssetID        string `json:"asset_id"`
	ClientOrderID  string `json:"client_order_id"`
	Status         string `json:"status"` // e.g. filled
	Type           string `json:"type"`   // e.g. limit, market
	Side           string `json:"side"`   // e.g. buy, sell
	Qty            string `json:"qty"`
	FilledQty      string `json:"filled_qty"`
	FilledAvgPrice string `json:"filled_avg_price"`
	LimitPrice     string `json:"limit_price"`
	CreatedAt      string `json:"created_at"`
	FilledAt       string `json:"filled_at"`
}

// MarketOrderBuy places a market buy order.
func (c *Client) MarketOrder(symbol string, qty decimal.Decimal) (*Order, error) {
	side := "buy"
	if qty.IsNegative() {
		side = "sell"
		qty = qty.Neg()
	}
	return c.CreateOrder(map[string]any{
		"symbol":        symbol,
		"qty":           qty.String(),
		"side":          side,
		"type":          "market",
		"time_in_force": "ioc",
	})
}

// LimitOrderBuy places a GTC limit buy order.
func (c *Client) LimitOrder(symbol string, side ds.Side, qty, limitPrice decimal.Decimal, strategy ds.LimitOrderStrategy) (*Order, error) {
	if qty.IsNegative() {
		return nil, errors.New("quantity must be positive for limit orders")
	}
	var time_in_force string
	switch strategy {
	case ds.LimitOrderStrategyMarketable:
		time_in_force = "gtc"
	case ds.LimitOrderStrategyIOC:
		time_in_force = "ioc"
	default:
		return nil, fmt.Errorf("%v orders not supported by Alpaca", strategy)
	}
	return c.CreateOrder(map[string]any{
		"symbol":        symbol,
		"qty":           qty.String(),
		"side":          side.String(),
		"type":          "limit",
		"time_in_force": time_in_force,
		"limit_price":   limitPrice.String(),
	})
}

func (c *Client) CreateOrder(body map[string]any) (*Order, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling order: %w", err)
	}
	resp, err := c.Request(ds.FastHTTPClient, "POST", "/v2/orders", bytes.NewReader(jsonBody))
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
	var result Order
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
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
	resp, err := c.Request(ds.FastHTTPClient, "PATCH", "/v2/orders/"+orderID, bytes.NewReader(jsonBody))
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
	resp, err := c.Request(ds.FastHTTPClient, "DELETE", "/v2/orders/"+orderID, nil)
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
	resp, err := c.Request(ds.FastHTTPClient, "DELETE", "/v2/orders", nil)
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

// GetOrders retrieves open orders.
func (c *Client) GetOrders() ([]Order, error) {
	resp, err := c.Request(ds.BulkHttpClient, "GET", "/v2/orders?status=open", nil)
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
