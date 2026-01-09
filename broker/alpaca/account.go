package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
)

// Account represents an Alpaca account.
// https://docs.alpaca.markets/reference/getaccount-1
type Account struct {
	ID                    string          `json:"id"`                          // e.g. 23aaaacc-0356-4699-8740-13fe6abcd62b
	AccountNumber         string          `json:"account_number"`              // e.g. 909499561
	CryptoStatus          string          `json:"crypto_status"`               // e.g. ACTIVE
	Currency              string          `json:"currency"`                    // e.g. USD
	CreatedAt             clocky.Time     `json:"created_at"`                  //
	Cash                  decimal.Decimal `json:"cash"`                        // cash balance
	BuyingPower           decimal.Decimal `json:"buying_power"`                // current available $ buying power; If multiplier = 4, this is your daytrade buying power which is calculated as (last_equity - (last) maintenance_margin) * 4; If multiplier = 2, buying_power = max(equity – initial_margin,0) * 2; If multiplier = 1, buying_power = cash
	RegTBuyingPower       decimal.Decimal `json:"regt_buying_power"`           // your buying power under Regulation T (your excess equity - equity minus margin value - times your margin multiplier)
	DaytradingBuyingPower decimal.Decimal `json:"daytrading_buying_power"`     // your buying power for day trades (continuously updated value)
	NonMarginBuyingPower  decimal.Decimal `json:"non_marginable_buying_power"` // current available non-margin dollar buying power
	IntradayAdjustments   decimal.Decimal `json:"intraday_adjustments"`        // the intraday adjustment by non_trade_activities such as fund deposit/withdraw.
	PendingRegTAFFees     decimal.Decimal `json:"pending_reg_taf_fees"`        // pending regulatory fees for the account
	AccruedFees           decimal.Decimal `json:"accrued_fees"`                // the fees collected
	Multiplier            decimal.Decimal `json:"multiplier"`                  // buying power multiplier that represents account margin classification; valid values 1 (standard limited margin account with 1x buying power), 2 (reg T margin account with 2x intraday and overnight buying power; this is the default for all non-PDT accounts with $2,000 or more equity), 4 (PDT account with 4x intraday buying power and 2x reg T overnight buying power)
	Equity                decimal.Decimal `json:"equity"`                      // cash + long_market_value + short_market_value
	LastEquity            decimal.Decimal `json:"last_equity"`                 // equity as of previous trading day at 16:00:00 ET
	LongMarketValue       decimal.Decimal `json:"long_market_value"`           // real-time MtM value of all long positions held in the account
	ShortMarketValue      decimal.Decimal `json:"short_market_value"`          // real-time MtM value of all short positions held in the account
	InitialMargin         decimal.Decimal `json:"initial_margin"`              // reg-t initial margin requirement (continuously updated value)
	MaintenanceMargin     decimal.Decimal `json:"maintenance_margin"`          // maintenance margin requirement (continuously updated value)
	LastMaintenanceMargin decimal.Decimal `json:"last_maintenance_margin"`     // your maintenance margin requirement on the previous trading day
	SMA                   decimal.Decimal `json:"sma"`                         // value of special memorandum account (will be used at a later date to provide additional buying_power)
	DaytradeCount         int64           `json:"daytrade_count"`              // the current number of daytrades that have been made in the last 5 trading days (inclusive of today)
	CryptoTier            int             `json:"crypto_tier"`                 //
	Status                AccountStatus   `json:"status"`                      //
	PatternDayTrader      bool            `json:"pattern_day_trader"`          //
	TradingBlocked        bool            `json:"trading_blocked"`             // account is not allowed to place orders
	TransfersBlocked      bool            `json:"transfers_blocked"`           // account is not allowed to request money transfers
	AccountBlocked        bool            `json:"account_blocked"`             // account activity by user is prohibited
	ShortingEnabled       bool            `json:"shorting_enabled"`            // whether or not the account is permitted to short
	TradeSuspendedByUser  bool            `json:"trade_suspended_by_user"`     //
}

// GetAccount retrieves account info.
func (c *Client) GetAccount() (*Account, error) {
	var result Account
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.BulkHttpClient, "GET", "/v2/account", nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
