package coinbase

import (
	"bytes"
	"dropbear/decimal"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// MovePortfolioFundsAmount is the amount to transfer.
type MovePortfolioFundsAmount struct {
	Value    string `json:"value"`    // amount to transfer
	Currency string `json:"currency"` // e.g., "USDC"
}

// MovePortfolioFundsRequest is the request body for moving funds between portfolios.
type MovePortfolioFundsRequest struct {
	Funds               MovePortfolioFundsAmount `json:"funds"`
	SourcePortfolioUUID string                   `json:"source_portfolio_uuid"`
	TargetPortfolioUUID string                   `json:"target_portfolio_uuid"`
}

// MovePortfolioFundsResponse is the response from moving funds.
type MovePortfolioFundsResponse struct {
	SourcePortfolioUUID string `json:"source_portfolio_uuid"`
	TargetPortfolioUUID string `json:"target_portfolio_uuid"`
	Errors              []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// MovePortfolioFunds transfers funds between portfolios.
func (c *Client) MovePortfolioFunds(currency string, amount decimal.Decimal, sourcePortfolioUUID, targetPortfolioUUID string) error {
	req := MovePortfolioFundsRequest{
		Funds: MovePortfolioFundsAmount{
			Value:    amount.String(),
			Currency: currency,
		},
		SourcePortfolioUUID: sourcePortfolioUUID,
		TargetPortfolioUUID: targetPortfolioUUID,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling move funds request: %w", err)
	}
	c.rateLimiter.Get()
	resp, err := c.Post("/api/v3/brokerage/portfolios/move_funds", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result MovePortfolioFundsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("decoding move funds response: %w (body: %s)", err, string(body))
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("move funds failed: %s", result.Errors[0].Message)
	}
	return nil
}

// ConvertQuoteRequest is the request body for creating a convert quote.
type ConvertQuoteRequest struct {
	FromAccount string `json:"from_account"` // account UUID
	ToAccount   string `json:"to_account"`   // account UUID
	Amount      string `json:"amount"`       // amount to convert
}

// ConvertAmount represents an amount with currency.
type ConvertAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// ConvertFee represents a fee in the conversion.
type ConvertFee struct {
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Amount      ConvertAmount `json:"amount"`
	Label       string        `json:"label"`
}

// ConvertTrade represents a conversion trade.
type ConvertTrade struct {
	ID                string        `json:"id"`
	Status            string        `json:"status"`
	UserEnteredAmount ConvertAmount `json:"user_entered_amount"`
	Amount            ConvertAmount `json:"amount"`
	Subtotal          ConvertAmount `json:"subtotal"`
	Total             ConvertAmount `json:"total"`
	Fees              []ConvertFee  `json:"fees"`
	TotalFee          ConvertAmount `json:"total_fee"`
	UnitPrice         struct {
		TargetToFiat   ConvertAmount `json:"target_to_fiat"`
		TargetToSource ConvertAmount `json:"target_to_source"`
		SourceToFiat   ConvertAmount `json:"source_to_fiat"`
	} `json:"unit_price"`
}

// ConvertQuoteResponse is the response from creating a convert quote.
type ConvertQuoteResponse struct {
	Trade  *ConvertTrade `json:"trade"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// CommitConvertRequest is the request body for committing a convert trade.
type CommitConvertRequest struct {
	FromAccount string `json:"from_account"`
	ToAccount   string `json:"to_account"`
}

// CommitConvertResponse is the response from committing a convert trade.
type CommitConvertResponse struct {
	Trade  *ConvertTrade `json:"trade"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// CreateConvertQuote creates a quote for converting between currencies.
// Returns the quote which must be committed with CommitConvertTrade.
func (c *Client) CreateConvertQuote(fromAccountID, toAccountID, amount string) (*ConvertTrade, error) {
	req := ConvertQuoteRequest{
		FromAccount: fromAccountID,
		ToAccount:   toAccountID,
		Amount:      amount,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling convert quote request: %w", err)
	}
	c.rateLimiter.Get()
	resp, err := c.Post("/api/v3/brokerage/convert/quote", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result ConvertQuoteResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding convert quote response: %w (body: %s)", err, string(body))
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("convert quote failed: %s", result.Errors[0].Message)
	}
	if result.Trade == nil {
		return nil, fmt.Errorf("convert quote returned nil trade (body: %s)", string(body))
	}
	return result.Trade, nil
}

// CommitConvertTrade commits a convert trade that was previously quoted.
func (c *Client) CommitConvertTrade(tradeID, fromAccountID, toAccountID string) (*ConvertTrade, error) {
	req := CommitConvertRequest{
		FromAccount: fromAccountID,
		ToAccount:   toAccountID,
	}
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling commit convert request: %w", err)
	}
	path := fmt.Sprintf("/api/v3/brokerage/convert/trade/%s", tradeID)
	c.rateLimiter.Get()
	resp, err := c.Post(path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result CommitConvertResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding commit convert response: %w (body: %s)", err, string(body))
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("commit convert failed: %s", result.Errors[0].Message)
	}
	if result.Trade == nil {
		return nil, fmt.Errorf("commit convert returned nil trade (body: %s)", string(body))
	}
	return result.Trade, nil
}

// ConvertUSDCtoUSD converts USDC to USD with zero fees.
// Panics if any fees would be charged.
func (c *Client) ConvertUSDCtoUSD(amount decimal.Decimal) (*ConvertTrade, error) {
	// Create quote using currency codes (not account UUIDs)
	quote, err := c.CreateConvertQuote("USDC", "USD", amount.String())
	if err != nil {
		return nil, fmt.Errorf("creating convert quote: %w", err)
	}

	// Verify zero fees
	if len(quote.Fees) > 0 {
		for _, fee := range quote.Fees {
			feeAmount := decimal.Parse(fee.Amount.Value)
			if !feeAmount.IsZero() {
				return nil, fmt.Errorf("unexpected fee for USDC->USD conversion: %s %s (%s)",
					fee.Amount.Value, fee.Amount.Currency, fee.Title)
			}
		}
	}
	if quote.TotalFee.Value != "" {
		totalFee := decimal.Parse(quote.TotalFee.Value)
		if !totalFee.IsZero() {
			return nil, fmt.Errorf("unexpected total fee for USDC->USD conversion: %s %s",
				quote.TotalFee.Value, quote.TotalFee.Currency)
		}
	}

	// Verify 1:1 conversion rate
	inputAmount := decimal.Parse(quote.UserEnteredAmount.Value)
	outputAmount := decimal.Parse(quote.Amount.Value)
	if inputAmount.Cmp(outputAmount) != 0 {
		return nil, fmt.Errorf("USDC->USD conversion not 1:1: input=%s output=%s",
			inputAmount, outputAmount)
	}

	// Commit the trade using currency codes
	trade, err := c.CommitConvertTrade(quote.ID, "USDC", "USD")
	if err != nil {
		return nil, fmt.Errorf("committing convert trade: %w", err)
	}

	return trade, nil
}
