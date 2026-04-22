package alpaca

import (
	"dropbear/decimal"
	"dropbear/netty"
	"fmt"
	"strings"
)

// Quote represents an NBBO quote.
type Quote struct {
	BidPrice       decimal.Decimal `json:"bp"` // bid price (0 means the security has no active bid)
	BidSize        decimal.Decimal `json:"bs"` // bid size in shares (round lots prior to November 3, 2025)
	BidExchange    string          `json:"bx"` // bid exchange (see v2/stocks/meta/exchanges for more details)
	AskPrice       decimal.Decimal `json:"ap"` // ask price (0 means the security has no active ask)
	AskSize        decimal.Decimal `json:"as"` // ask size in shares (round lots prior to November 3, 2025)
	AskExchange    string          `json:"ax"` // ask exchange (see v2/stocks/meta/exchanges for more details)
	ConditionFlags []string        `json:"c"`  // condition flags (see v2/stocks/meta/conditions/quote for more details. If the array contains one flag, it applies to both the bid and ask. If the array contains two flags, the first one applies to the bid and the second one to the ask)
	Exchange       string          `json:"z"`  // some exchange thing
}

// GetQuote fetches the latest NBBO quote for a stock symbol.
// https://docs.alpaca.markets/reference/stocklatestquotesingle-1
func (c *Client) GetQuote(sym string) (*Quote, error) {
	url := fmt.Sprintf("https://%s/v2/stocks/%s/quotes/latest", DataHost, sym)
	var result struct {
		Quote  *Quote `json:"quote"`
		Symbol string `json:"symbol"`
	}
	c.DataTokenBucket.Get()
	err := c.RequestJSON(netty.FastHTTPClient, "GET", url, false, nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Quote, nil
}

// GetQuotes fetches the latest NBBO quotes for multiple stock symbols.
// https://docs.alpaca.markets/reference/stocklatestquotes-1
func (c *Client) GetQuotes(symbols []string) (map[string]*Quote, error) {
	if len(symbols) == 0 {
		return nil, nil
	}
	var url strings.Builder
	fmt.Fprintf(&url, "https://%s/v2/stocks/quotes/latest?symbols=", DataHost)
	for i, sym := range symbols {
		if i > 0 {
			url.WriteString(",")
		}
		url.WriteString(sym)
	}
	var result struct {
		Quotes map[string]*Quote `json:"quotes"`
		Symbol string            `json:"symbol"`
	}
	c.DataTokenBucket.Get()
	err := c.RequestJSON(netty.BulkHttpClient, "GET", url.String(), false, nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Quotes, nil
}

// GetCryptoQuote fetches the latest quote for a crypto symbol.
// https://docs.alpaca.markets/reference/cryptolatestquotes-1
func (c *Client) GetCryptoQuote(sym string) (*Quote, error) {
	url := fmt.Sprintf("https://%s/v1beta3/crypto/us/latest/quotes?symbols=%s", DataHost, sym)
	var result struct {
		Quotes map[string]*Quote `json:"quotes"`
	}
	c.DataTokenBucket.Get()
	err := c.RequestJSON(netty.FastHTTPClient, "GET", url, false, nil, &result)
	if err != nil {
		return nil, err
	}
	quote, ok := result.Quotes[sym]
	if !ok {
		return nil, fmt.Errorf("no quote found for %s", sym)
	}
	return quote, nil
}
