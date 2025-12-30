package coinbase

import (
	"bytes"
	"dropbear/ds"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
)

// SendCryptoRequest is the request body for sending crypto.
type SendCryptoRequest struct {
	Type        string `json:"type"`                  // must be "send"
	To          string `json:"to"`                    // blockchain address
	Amount      string `json:"amount"`                // amount to send
	Currency    string `json:"currency"`              // e.g., "SOL"
	Idem        string `json:"idem,omitempty"`        // idempotency key
	Network     string `json:"network,omitempty"`     // e.g., "solana"
	Description string `json:"description,omitempty"` // optional memo
}

// SendCryptoResponse is the response from sending crypto.
type SendCryptoResponse struct {
	Data struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Status string `json:"status"` // "pending" or "completed"
		Amount struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
		} `json:"amount"`
		NativeAmount struct {
			Amount   string `json:"amount"`
			Currency string `json:"currency"`
		} `json:"native_amount"`
		Network struct {
			Status string `json:"status"`
			Hash   string `json:"hash"`
			Name   string `json:"name"`
		} `json:"network"`
		To struct {
			Resource string `json:"resource"`
			Address  string `json:"address"`
		} `json:"to"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	} `json:"data"`
	Errors []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"errors"`
}

// SendCrypto sends cryptocurrency to an external address.
// accountID is the UUID of the source account (get from GetAccounts).
func (c *Client) SendCrypto(accountID string, to string, amount string, currency string, network string) (*SendCryptoResponse, error) {
	req := SendCryptoRequest{
		Type:     "send",
		To:       to,
		Amount:   amount,
		Currency: currency,
		Idem:     uuid.New().String(),
		Network:  network,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling send request: %w", err)
	}
	path := fmt.Sprintf("/v2/accounts/%s/transactions", accountID)
	c.rateLimiter.Get()
	resp, err := c.Request(ds.BulkHttpClient, "POST", path, bytes.NewReader(jsonBody), true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result SendCryptoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w (body: %s)", err, string(body))
	}
	if len(result.Errors) > 0 {
		return &result, fmt.Errorf("send failed: %s", result.Errors[0].Message)
	}
	return &result, nil
}
