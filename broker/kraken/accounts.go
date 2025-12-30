package kraken

import (
	"dropbear/decimal"
	"net/url"
	"strings"
)

// Balance represents the balance response from Kraken.
// Keys are asset names (e.g., "XXBT", "ZUSD", "ETH").
type Balance map[string]string

// GetBalance retrieves all account balances.
func (c *Client) GetBalance() (Balance, error) {
	c.restLimiter.Get()
	values := url.Values{}
	resp, err := c.Post("/0/private/Balance", values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result Balance
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ExtendedBalance represents extended balance information.
type ExtendedBalance struct {
	Balance    string `json:"balance"`
	Credit     string `json:"credit"`
	CreditUsed string `json:"credit_used"`
	HoldTrade  string `json:"hold_trade"`
}

// ExtendedBalanceResponse is the response from the BalanceEx endpoint.
type ExtendedBalanceResponse map[string]ExtendedBalance

// GetExtendedBalance retrieves all extended account balances including credits and holds.
// Available balance = balance + credit - credit_used - hold_trade
func (c *Client) GetExtendedBalance() (ExtendedBalanceResponse, error) {
	c.restLimiter.Get()
	values := url.Values{}
	resp, err := c.Post("/0/private/BalanceEx", values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result ExtendedBalanceResponse
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TradeBalance represents the trade balance response.
type TradeBalance struct {
	EquivalentBalance string `json:"eb"` // combined balance of all currencies
	TradeBalance      string `json:"tb"` // combined balance of all equity currencies
	MarginAmount      string `json:"m"`  // margin amount of open positions
	UnrealizedPnL     string `json:"n"`  // unrealized net profit/loss of open positions
	CostBasis         string `json:"c"`  // cost basis of open positions
	CurrentValue      string `json:"v"`  // current floating valuation of open positions
	Equity            string `json:"e"`  // trade balance + unrealized net profit/loss
	FreeMargin        string `json:"mf"` // equity - initial margin (maximum margin available)
	MarginLevel       string `json:"ml"` // (equity / initial margin) * 100
}

// GetTradeBalance retrieves trade balance summary.
// Pass empty string for asset to get balance in ZUSD (default).
func (c *Client) GetTradeBalance(asset string) (*TradeBalance, error) {
	c.restLimiter.Get()
	values := url.Values{}
	if asset != "" {
		values.Set("asset", asset)
	}
	resp, err := c.Post("/0/private/TradeBalance", values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result TradeBalance
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetBalanceDecimal returns the balance for a specific asset as a Decimal.
func (b Balance) GetBalanceDecimal(asset string) decimal.Decimal {
	if val, ok := b[asset]; ok {
		return decimal.Parse(val)
	}
	return decimal.Zero
}

// TradeVolumeResult is the response from the TradeVolume endpoint.
type TradeVolumeResult struct {
	Currency  string                     `json:"currency"`   // volume currency (e.g. "ZUSD")
	Volume    string                     `json:"volume"`     // 30-day trading volume
	Fees      map[string]TradeVolumeFees `json:"fees"`       // taker fees per pair
	FeesMaker map[string]TradeVolumeFees `json:"fees_maker"` // maker fees per pair
}

// TradeVolumeFees contains fee info for a specific pair.
type TradeVolumeFees struct {
	Fee        string `json:"fee"`        // current fee rate as percent (e.g. "0.2600")
	MinFee     string `json:"minfee"`     // minimum fee rate
	MaxFee     string `json:"maxfee"`     // maximum fee rate
	NextFee    string `json:"nextfee"`    // fee rate at next volume tier
	NextVolume string `json:"nextvolume"` // volume needed for next tier
	TierVolume string `json:"tiervolume"` // volume threshold for current tier
}

// GetTradeVolume retrieves 30-day trading volume and fee rates for specified pairs.
// Pass one or more pairs (e.g. "XBTUSD", "ETHUSD") to get their fee schedules.
// The returned fees are percentages (e.g. "0.26" means 0.26%).
// https://docs.kraken.com/api/docs/rest-api/get-trade-volume
func (c *Client) GetTradeVolume(pairs ...string) (*TradeVolumeResult, error) {
	c.restLimiter.Get()
	values := url.Values{}
	if len(pairs) > 0 {
		values.Set("pair", strings.Join(pairs, ","))
	}
	resp, err := c.Post("/0/private/TradeVolume", values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result TradeVolumeResult
	if err := c.decodeResponse(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FeeRates returns the current maker and taker fee rates as decimals (e.g. 0.0026 for 0.26%).
// Pass a pair like "XBTUSD" to get fees for that specific pair.
func (c *Client) FeeRates(pair string) (maker, taker decimal.Decimal, err error) {
	vol, err := c.GetTradeVolume(pair)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	if fees, ok := vol.Fees[pair]; ok {
		// Fee is returned as percent string like "0.26" meaning 0.26%
		// Convert to decimal by dividing by 100
		taker = decimal.Parse(fees.Fee).DivInt(100)
	}
	if fees, ok := vol.FeesMaker[pair]; ok {
		maker = decimal.Parse(fees.Fee).DivInt(100)
	}
	return maker, taker, nil
}
