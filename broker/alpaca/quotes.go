package alpaca

import (
	"dropbear/decimal"
	"dropbear/ds/symbol"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
func (c *Client) GetQuote(sym symbol.Symbol) (*Quote, error) {
	c.DataTokenBucket.Get()
	resp, err := c.Get(fmt.Sprintf("https://%s/v2/stocks/%s/quotes/latest", DataHost, sym))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Quote  *Quote `json:"quote"`
		Symbol string `json:"symbol"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Quote, nil
}

// GetCryptoQuote fetches the latest quote for a crypto symbol.
// Symbol should be in Alpaca format (e.g., "BTCUSD") - it will be converted to "BTC/USD".
// https://docs.alpaca.markets/reference/cryptolatestquotes-1
func (c *Client) GetCryptoQuote(sym symbol.Symbol) (*Quote, error) {
	c.DataTokenBucket.Get()
	// Convert BTCUSD -> BTC/USD format for the API
	s := sym.String()
	apiSymbol := s
	if len(s) >= 6 && s[len(s)-3:] == "USD" {
		apiSymbol = s[:len(s)-3] + "/USD"
	}
	resp, err := c.Get(fmt.Sprintf("https://%s/v1beta3/crypto/us/latest/quotes?symbols=%s", DataHost, apiSymbol))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Quotes map[string]*Quote `json:"quotes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	quote, ok := result.Quotes[apiSymbol]
	if !ok {
		return nil, fmt.Errorf("no quote found for %s", sym)
	}
	return quote, nil
}
