package coinbase

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Portfolio represents a Coinbase portfolio.
type Portfolio struct {
	Name    string `json:"name"`
	UUID    string `json:"uuid"`
	Type    string `json:"type"`
	Deleted bool   `json:"deleted"`
}

// SpotPosition represents a spot position in a portfolio.
type SpotPosition struct {
	Asset                     string         `json:"asset"` // e.g. USD, BTC
	AccountUUID               string         `json:"account_uuid"`
	AccountType               string         `json:"account_type"` // e.g. ACCOUNT_TYPE_FIAT, ACCOUNT_TYPE_CRYPTO
	TotalBalanceFiat          json.Number    `json:"total_balance_fiat"`
	TotalBalanceCrypto        json.Number    `json:"total_balance_crypto"`
	AvailableToTradeFiat      json.Number    `json:"available_to_trade_fiat"`
	AvailableToTradeCrypto    json.Number    `json:"available_to_trade_crypto"`
	AvailableToTransferCrypto json.Number    `json:"available_to_transfer_crypto"`
	AvailableToSendCrypto     json.Number    `json:"available_to_send_crypto"`
	IsCash                    bool           `json:"is_cash"`
	AverageEntryPrice         MonetaryAmount `json:"average_entry_price"`
}

// PortfolioBreakdown is the response from the portfolio breakdown endpoint.
type PortfolioBreakdown struct {
	Breakdown struct {
		SpotPositions []SpotPosition `json:"spot_positions"`
	} `json:"breakdown"`
}

// PortfoliosResponse is the response from the portfolios endpoint.
type PortfoliosResponse struct {
	Portfolios []Portfolio `json:"portfolios"`
}

// GetPortfolios retrieves all portfolios.
func (c *Client) GetPortfolios() (*PortfoliosResponse, error) {
	c.rateLimiter.Get()
	resp, err := c.Get("/api/v3/brokerage/portfolios")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result PortfoliosResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// GetPortfolioBreakdown retrieves the breakdown for a portfolio.
func (c *Client) GetPortfolioBreakdown(uuid string) (*PortfolioBreakdown, error) {
	c.rateLimiter.Get()
	resp, err := c.Get("/api/v3/brokerage/portfolios/" + uuid)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result PortfolioBreakdown
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
