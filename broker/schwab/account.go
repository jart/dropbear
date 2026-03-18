package schwab

import (
	"dropbear/decimal"
	"dropbear/netty"
	"fmt"
)

// AccountResponse wraps the top-level account response from Schwab.
type AccountResponse struct {
	SecuritiesAccount SecuritiesAccount `json:"securitiesAccount"`
}

// SecuritiesAccount represents a Schwab securities account.
type SecuritiesAccount struct {
	Type                    string          `json:"type"`                    // e.g. "MARGIN"
	AccountNumber           string          `json:"accountNumber"`           // e.g. "40595135"
	RoundTrips              int64           `json:"roundTrips"`              // e.g. 0
	IsDayTrader             bool            `json:"isDayTrader"`             // means you can tap day trading buying power
	IsClosingOnlyRestricted bool            `json:"isClosingOnlyRestricted"` // means you're in a margin call
	PfcbFlag                bool            `json:"pfcbFlag"`                // portfolio cash balance flag; if true, cash balance is swept to money market fund and not available for trading
	Positions               []Position      `json:"positions"`               // list of open positions in the account
	InitialBalances         Balances        `json:"initialBalances"`         // initial account balances at the start of the day
	CurrentBalances         CurrentBalances `json:"currentBalances"`
	ProjectedBalances       CurrentBalances `json:"projectedBalances"`
}

// Position represents a position in an account.
type Position struct {
	LongQuantity                 decimal.Decimal `json:"longQuantity"`  // e.g. 100 for 100 shares long, or 0 if no long position
	ShortQuantity                decimal.Decimal `json:"shortQuantity"` // e.g. 500 for 500 shares short, or 0 if no short position
	SettledLongQuantity          decimal.Decimal `json:"settledLongQuantity"`
	SettledShortQuantity         decimal.Decimal `json:"settledShortQuantity"`
	AveragePrice                 decimal.Decimal `json:"averagePrice"`
	CurrentDayProfitLoss         decimal.Decimal `json:"currentDayProfitLoss"`
	CurrentDayProfitLossPercent  decimal.Decimal `json:"currentDayProfitLossPercentage"`
	AgedQuantity                 decimal.Decimal `json:"agedQuantity"`
	Instrument                   Instrument      `json:"instrument"`
	MarketValue                  decimal.Decimal `json:"marketValue"`
	MaintenanceRequirement       decimal.Decimal `json:"maintenanceRequirement"`
	AverageLongPrice             decimal.Decimal `json:"averageLongPrice"`
	AverageShortPrice            decimal.Decimal `json:"averageShortPrice"`
	TaxLotAverageLongPrice       decimal.Decimal `json:"taxLotAverageLongPrice"`
	TaxLotAverageShortPrice      decimal.Decimal `json:"taxLotAverageShortPrice"`
	LongOpenProfitLoss           decimal.Decimal `json:"longOpenProfitLoss"`
	ShortOpenProfitLoss          decimal.Decimal `json:"shortOpenProfitLoss"`
	PreviousSessionLongQuantity  decimal.Decimal `json:"previousSessionLongQuantity"`
	PreviousSessionShortQuantity decimal.Decimal `json:"previousSessionShortQuantity"`
	CurrentDayCost               decimal.Decimal `json:"currentDayCost"`
}

// Balances represents initial account balances.
type Balances struct {
	AccruedInterest                  decimal.Decimal `json:"accruedInterest"`
	AvailableFundsNonMarginableTrade decimal.Decimal `json:"availableFundsNonMarginableTrade"`
	BondValue                        decimal.Decimal `json:"bondValue"`
	BuyingPower                      decimal.Decimal `json:"buyingPower"`
	CashBalance                      decimal.Decimal `json:"cashBalance"`
	CashAvailableForTrading          decimal.Decimal `json:"cashAvailableForTrading"`
	CashReceipts                     decimal.Decimal `json:"cashReceipts"`
	DayTradingBuyingPower            decimal.Decimal `json:"dayTradingBuyingPower"`
	DayTradingBuyingPowerCall        decimal.Decimal `json:"dayTradingBuyingPowerCall"`
	DayTradingEquityCall             decimal.Decimal `json:"dayTradingEquityCall"`
	Equity                           decimal.Decimal `json:"equity"`
	EquityPercentage                 decimal.Decimal `json:"equityPercentage"`
	LiquidationValue                 decimal.Decimal `json:"liquidationValue"`
	LongMarginValue                  decimal.Decimal `json:"longMarginValue"`
	LongOptionMarketValue            decimal.Decimal `json:"longOptionMarketValue"`
	LongStockValue                   decimal.Decimal `json:"longStockValue"`
	MaintenanceCall                  decimal.Decimal `json:"maintenanceCall"`
	MaintenanceRequirement           decimal.Decimal `json:"maintenanceRequirement"`
	Margin                           decimal.Decimal `json:"margin"`
	MarginEquity                     decimal.Decimal `json:"marginEquity"`
	MoneyMarketFund                  decimal.Decimal `json:"moneyMarketFund"`
	MutualFundValue                  decimal.Decimal `json:"mutualFundValue"`
	RegTCall                         decimal.Decimal `json:"regTCall"`
	ShortMarginValue                 decimal.Decimal `json:"shortMarginValue"`
	ShortOptionMarketValue           decimal.Decimal `json:"shortOptionMarketValue"`
	ShortStockValue                  decimal.Decimal `json:"shortStockValue"`
	TotalCash                        decimal.Decimal `json:"totalCash"`
	IsInCall                         decimal.Decimal `json:"isInCall"`
	UnsettledCash                    decimal.Decimal `json:"unsettledCash"`
	PendingDeposits                  decimal.Decimal `json:"pendingDeposits"`
	MarginBalance                    decimal.Decimal `json:"marginBalance"`
	ShortBalance                     decimal.Decimal `json:"shortBalance"`
	AccountValue                     decimal.Decimal `json:"accountValue"`
}

// CurrentBalances represents current or projected account balances.
type CurrentBalances struct {
	AvailableFunds                   decimal.Decimal `json:"availableFunds"`
	AvailableFundsNonMarginableTrade decimal.Decimal `json:"availableFundsNonMarginableTrade"`
	BuyingPower                      decimal.Decimal `json:"buyingPower"`
	BuyingPowerNonMarginableTrade    decimal.Decimal `json:"buyingPowerNonMarginableTrade"`
	DayTradingBuyingPower            decimal.Decimal `json:"dayTradingBuyingPower"`
	DayTradingBuyingPowerCall        decimal.Decimal `json:"dayTradingBuyingPowerCall"`
	Equity                           decimal.Decimal `json:"equity"`
	EquityPercentage                 decimal.Decimal `json:"equityPercentage"`
	LongMarginValue                  decimal.Decimal `json:"longMarginValue"`
	MaintenanceCall                  decimal.Decimal `json:"maintenanceCall"`
	MaintenanceRequirement           decimal.Decimal `json:"maintenanceRequirement"`
	MarginBalance                    decimal.Decimal `json:"marginBalance"`
	RegTCall                         decimal.Decimal `json:"regTCall"`
	ShortBalance                     decimal.Decimal `json:"shortBalance"`
	ShortMarginValue                 decimal.Decimal `json:"shortMarginValue"`
	SMA                              decimal.Decimal `json:"sma"`
	IsInCall                         decimal.Decimal `json:"isInCall"`
	StockBuyingPower                 decimal.Decimal `json:"stockBuyingPower"`
	OptionBuyingPower                decimal.Decimal `json:"optionBuyingPower"`
}

// LinkedAccount maps a plain account number to its hash value.
// Schwab requires the hash value (not the plain number) in all URL paths.
type LinkedAccount struct {
	AccountNumber string `json:"accountNumber"`
	HashValue     string `json:"hashValue"`
}

// GetAccountNumbers returns account numbers and their hash values.
// The hash values must be used in all API URL paths instead of plain account numbers.
func (c *Client) GetAccountNumbers() ([]LinkedAccount, error) {
	var result []LinkedAccount
	err := c.RequestJSON(netty.BulkHttpClient, "GET", "/accounts/accountNumbers", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetAccounts retrieves all accounts linked to the authenticated user, including positions.
func (c *Client) GetAccounts() ([]AccountResponse, error) {
	var result []AccountResponse
	err := c.RequestJSON(netty.BulkHttpClient, "GET", "/accounts?fields=positions", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetAccount retrieves a single account by hash, including positions.
func (c *Client) GetAccount() (*AccountResponse, error) {
	token := getToken()
	var result AccountResponse
	err := c.RequestJSON(netty.BulkHttpClient, "GET",
		fmt.Sprintf("/accounts/%s?fields=positions", token.AccountHash),
		nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// SetAccount discovers the first linked account and caches its hash value.
// Returns the account hash which is needed for all order and account-specific API calls.
// This must be called before placing orders if the account hash is not already known.
func (c *Client) SetAccount() (string, error) {
	t := getToken()
	if t != nil && t.AccountHash != "" {
		return t.AccountHash, nil
	}
	accounts, err := c.GetAccountNumbers()
	if err != nil {
		return "", err
	}
	if len(accounts) == 0 {
		return "", fmt.Errorf("schwab: no linked accounts found")
	}
	if t == nil {
		return "", fmt.Errorf("schwab: no token found; authorize first")
	}
	t.AccountHash = accounts[0].HashValue
	t.AccountNumber = accounts[0].AccountNumber
	setToken(t)
	return t.AccountHash, nil
}
