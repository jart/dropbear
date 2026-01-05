package coinbase

import (
	"dropbear/netty"
	"encoding/json"
	"fmt"
	"io"
)

// V2Address represents a Coinbase deposit address.
type V2Address struct {
	ID          string `json:"id"`
	Address     string `json:"address"`
	AddressInfo *struct {
		Address string `json:"address"`
		Tag     string `json:"destination_tag,omitempty"`
	} `json:"address_info,omitempty"`
	Name         string `json:"name"`
	Network      string `json:"network"`
	URIScheme    string `json:"uri_scheme,omitempty"`
	Resource     string `json:"resource"`
	ResourcePath string `json:"resource_path"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	DepositURI   string `json:"deposit_uri,omitempty"`
	CallbackURL  string `json:"callback_url,omitempty"`
}

// v2AddressesResponse is the response from the addresses endpoint.
type v2AddressesResponse struct {
	Data       []V2Address `json:"data"`
	Pagination struct {
		NextURI string `json:"next_uri"`
	} `json:"pagination"`
}

// GetDepositAddresses fetches deposit addresses for an account.
// accountID is the v2 account UUID (get from GetV2AccountByCurrencyCode).
// https://docs.cdp.coinbase.com/coinbase-business/track-apis/addresses
func (c *Client) GetDepositAddresses(accountID string) ([]V2Address, error) {
	path := fmt.Sprintf("/v2/accounts/%s/addresses", accountID)
	var addresses []V2Address
	for path != "" {
		c.rateLimiter.Get()
		resp, err := c.Get(path)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("get addresses error %d: %s", resp.StatusCode, string(body))
		}
		var result v2AddressesResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decoding addresses: %w", err)
		}
		addresses = append(addresses, result.Data...)
		path = result.Pagination.NextURI
	}
	return addresses, nil
}

// CreateDepositAddress creates a new deposit address for an account.
// accountID is the v2 account UUID.
// https://docs.cdp.coinbase.com/coinbase-business/track-apis/addresses#create-address
func (c *Client) CreateDepositAddress(accountID string) (*V2Address, error) {
	path := fmt.Sprintf("/v2/accounts/%s/addresses", accountID)
	c.rateLimiter.Get()
	resp, err := c.Request(netty.BulkHttpClient, "POST", path, nil, true)
	if err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		return nil, fmt.Errorf("create address error %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Data V2Address `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding address: %w", err)
	}
	return &result.Data, nil
}
