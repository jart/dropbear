package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
	"fmt"
	"net/url"
)

// OptionGreeks represents the Greeks for an option contract.
type OptionGreeks struct {
	Delta decimal.Decimal `json:"delta"` // rate of change of option price with respect to underlying
	Gamma decimal.Decimal `json:"gamma"` // rate of change of delta with respect to underlying
	Theta decimal.Decimal `json:"theta"` // rate of change of option price with respect to time
	Vega  decimal.Decimal `json:"vega"`  // rate of change of option price with respect to volatility
	Rho   decimal.Decimal `json:"rho"`   // rate of change of option price with respect to interest rate
}

// OptionTrade represents a single option trade execution.
type OptionTrade struct {
	Timestamp clocky.Time     `json:"t"` // RFC-3339 timestamp
	Price     decimal.Decimal `json:"p"` // execution price
	Size      int64           `json:"s"` // quantity traded
	Exchange  string          `json:"x"` // exchange code
	Condition string          `json:"c"` // trade condition
}

// OptionQuote represents an option quote.
type OptionQuote struct {
	Timestamp   clocky.Time     `json:"t"`  // RFC-3339 timestamp
	BidPrice    decimal.Decimal `json:"bp"` // bid price
	BidSize     int64           `json:"bs"` // bid size
	BidExchange string          `json:"bx"` // bid exchange
	AskPrice    decimal.Decimal `json:"ap"` // ask price
	AskSize     int64           `json:"as"` // ask size
	AskExchange string          `json:"ax"` // ask exchange
}

// OptionSnapshot represents a point-in-time view of an option contract.
type OptionSnapshot struct {
	LatestTrade       *OptionTrade    `json:"latestTrade"`
	LatestQuote       *OptionQuote    `json:"latestQuote"`
	Greeks            *OptionGreeks   `json:"greeks"`
	ImpliedVolatility decimal.Decimal `json:"impliedVolatility"`
}

// OptionChainRequest specifies parameters for the option chain request.
type OptionChainRequest struct {
	Feed          DataFeed        // data feed (sip, indicative)
	Type          string          // "put" or "call"
	StrikeGTE     decimal.Decimal // minimum strike price
	StrikeLTE     decimal.Decimal // maximum strike price
	ExpirationGTE clocky.Time     // minimum expiration date
	ExpirationLTE clocky.Time     // maximum expiration date
	RootSymbol    string          // root symbol filter
	Limit         int             // max contracts to return (default 100)
	PageToken     string          // pagination token
}

// GetOptionChain fetches the option chain for an underlying symbol.
// Returns a map of option contract symbol to snapshot data including
// latest trade, latest quote, and greeks, plus the next page token.
// https://docs.alpaca.markets/reference/optionchain
func (c *Client) GetOptionChain(underlying string, req *OptionChainRequest) (map[string]*OptionSnapshot, string, error) {
	u := &url.URL{
		Scheme: "https",
		Host:   DataHost,
		Path:   "/v1beta1/options/snapshots/" + underlying,
	}
	if req != nil {
		q := u.Query()
		if req.Feed != 0 {
			q.Set("feed", req.Feed.String())
		}
		if req.Type != "" {
			q.Set("type", req.Type)
		}
		if !req.StrikeGTE.IsZero() {
			q.Set("strike_price_gte", req.StrikeGTE.String())
		}
		if !req.StrikeLTE.IsZero() {
			q.Set("strike_price_lte", req.StrikeLTE.String())
		}
		if req.ExpirationGTE != 0 {
			q.Set("expiration_date_gte", req.ExpirationGTE.RFC3339()[:10])
		}
		if req.ExpirationLTE != 0 {
			q.Set("expiration_date_lte", req.ExpirationLTE.RFC3339()[:10])
		}
		if req.RootSymbol != "" {
			q.Set("root_symbol", req.RootSymbol)
		}
		if req.Limit > 0 {
			q.Set("limit", fmt.Sprintf("%d", req.Limit))
		}
		if req.PageToken != "" {
			q.Set("page_token", req.PageToken)
		}
		u.RawQuery = q.Encode()
	}
	var result struct {
		Snapshots     map[string]*OptionSnapshot `json:"snapshots"`
		NextPageToken string                     `json:"next_page_token"`
	}
	c.DataTokenBucket.Get()
	err := c.RequestJSON(netty.BulkHttpClient, "GET", u.String(), nil, &result)
	if err != nil {
		return nil, "", err
	}
	return result.Snapshots, result.NextPageToken, nil
}

// GetOptionSnapshot fetches a snapshot for a single option contract symbol.
// https://docs.alpaca.markets/reference/optionsnapshot
func (c *Client) GetOptionSnapshot(symbol string) (*OptionSnapshot, error) {
	url := fmt.Sprintf("https://%s/v1beta1/options/snapshots?symbols=%s", DataHost, symbol)
	var result struct {
		Snapshots map[string]*OptionSnapshot `json:"snapshots"`
	}
	c.DataTokenBucket.Get()
	err := c.RequestJSON(netty.FastHTTPClient, "GET", url, nil, &result)
	if err != nil {
		return nil, err
	}
	snap, ok := result.Snapshots[symbol]
	if !ok {
		return nil, fmt.Errorf("no snapshot found for %s", symbol)
	}
	return snap, nil
}
