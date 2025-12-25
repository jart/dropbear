package alpaca

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Account represents the account info.
type Account struct {
	ID                       string `json:"id"`
	AccountNumber            string `json:"account_number"`
	Status                   string `json:"status"`                 // e.g. ACTIVE
	CryptoStatus             string `json:"crypto_status"`          // e.g. ACTIVE
	OptionsApprovedLevel     int    `json:"options_approved_level"` // e.g. 3
	OptionsTradingLevel      int    `json:"options_trading_level"`  // e.g. 3
	Currency                 string `json:"currency"`
	Cash                     string `json:"cash"`
	BuyingPower              string `json:"buying_power"`
	RegTBuyingPower          string `json:"regt_buying_power"`
	DayTradingBuyingPower    string `json:"daytrading_buying_power"`
	EffectiveBuyingPower     string `json:"effective_buying_power"`
	NonMarginableBuyingPower string `json:"non_marginable_buying_power"`
	OptionsBuyingPower       string `json:"options_buying_power"`
	PatternDayTrader         bool   `json:"pattern_day_trader"`
	BodDtbp                  string `json:"bod_dtbp"`
	Equity                   string `json:"equity"`
	LastEquity               string `json:"last_equity"`
	AccruedFees              string `json:"accrued_fees"`
	PortfolioValue           string `json:"portfolio_value"`
	LongMarketValue          string `json:"long_market_value"`
	ShortMarketValue         string `json:"short_market_value"`
	PositionMarketValue      string `json:"position_market_value"`
	TradingBlocked           bool   `json:"trading_blocked"`
	AccountBlocked           bool   `json:"account_blocked"`
	TradeSuspendedByUser     bool   `json:"trade_suspended_by_user"`
	CreatedAt                string `json:"created_at"`
	Multiplier               string `json:"multiplier"` // e.g. 4
	ShortingEnabled          bool   `json:"shorting_enabled"`
	InitialMargin            string `json:"initial_margin"`
	MaintenanceMargin        string `json:"maintenance_margin"`
	LastMaintenanceMargin    string `json:"last_maintenance_margin"`
	Sma                      string `json:"sma"`
	DayTradeCount            int    `json:"daytrade_count"`
	BalanceAsof              string `json:"balance_asof"`
	CryptoTier               int    `json:"crypto_tier"` // e.g. 1
	IntradayAdjustments      string `json:"intraday_adjustments"`
	PendingRegTafFees        string `json:"pending_reg_taf_fees"`
}

// GetAccount retrieves account info.
func (c *Client) GetAccount() (*Account, error) {
	resp, err := c.Get("/v2/account")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result Account
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}
