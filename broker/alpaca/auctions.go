package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
	"fmt"
	"net/url"
	"strings"
)

// AuctionPrice represents a single auction execution.
type AuctionPrice struct {
	Timestamp clocky.Time     `json:"t"` // RFC-3339 timestamp
	Price     decimal.Decimal `json:"p"` // auction price
	Size      int64           `json:"s"` // auction size
	Exchange  string          `json:"x"` // exchange code
	Condition string          `json:"c"` // auction condition
}

// Auction represents the opening and closing auctions for a trading day.
type Auction struct {
	Date    string         `json:"d"` // date in YYYY-MM-DD format
	Opening []AuctionPrice `json:"o"` // opening auction prices
	Closing []AuctionPrice `json:"c"` // closing auction prices
}

// GetAuctions fetches historical auction data for a stock symbol.
// Returns auctions and the next page token (empty if no more pages).
// start defaults to the beginning of the current day.
// end defaults to the current time.
// limit defaults to 1000 if 0 is passed and the maximum is 10000.
// https://docs.alpaca.markets/reference/stockauctionsingle-1
func (c *Client) GetAuctions(sym string, start, end clocky.Time, feed DataFeed, limit int, desc bool, pageToken string) ([]Auction, string, error) {
	if limit < 0 || limit > 10000 {
		return nil, "", ErrBarsUnsupportedLimit
	}
	c.DataTokenBucket.Get()
	u := &url.URL{
		Scheme: "https",
		Host:   DataHost,
		Path:   "/v2/stocks/" + sym + "/auctions",
	}
	q := u.Query()
	if start != 0 {
		q.Set("start", start.RFC3339())
	}
	if end != 0 {
		q.Set("end", end.RFC3339())
	}
	if feed != 0 {
		q.Set("feed", feed.String())
	}
	if limit != 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	if desc {
		q.Set("sort", "desc")
	}
	u.RawQuery = q.Encode()
	var result struct {
		Auctions  []Auction `json:"auctions"`
		NextToken string    `json:"next_page_token"`
	}
	err := c.RequestJSON(netty.BulkHttpClient, "GET", u.String(), false, nil, &result)
	if err != nil {
		return nil, "", err
	}
	return result.Auctions, result.NextToken, nil
}

// GetAuctionsForSymbols fetches historical auction data for multiple stock symbols.
// Returns a map of symbol to auctions and the next page token.
// https://docs.alpaca.markets/reference/stockauctions
func (c *Client) GetAuctionsForSymbols(symbols []string, start, end clocky.Time, feed DataFeed, limit int, desc bool, pageToken string) (map[string][]Auction, string, error) {
	if len(symbols) == 0 {
		return nil, "", nil
	}
	if limit < 0 || limit > 10000 {
		return nil, "", ErrBarsUnsupportedLimit
	}
	c.DataTokenBucket.Get()
	u := &url.URL{
		Scheme: "https",
		Host:   DataHost,
		Path:   "/v2/stocks/auctions",
	}
	q := u.Query()
	q.Set("symbols", strings.Join(symbols, ","))
	if start != 0 {
		q.Set("start", start.RFC3339())
	}
	if end != 0 {
		q.Set("end", end.RFC3339())
	}
	if feed != 0 {
		q.Set("feed", feed.String())
	}
	if limit != 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if pageToken != "" {
		q.Set("page_token", pageToken)
	}
	if desc {
		q.Set("sort", "desc")
	}
	u.RawQuery = q.Encode()
	var result struct {
		Auctions  map[string][]Auction `json:"auctions"`
		NextToken string               `json:"next_page_token"`
	}
	err := c.RequestJSON(netty.BulkHttpClient, "GET", u.String(), false, nil, &result)
	if err != nil {
		return nil, "", err
	}
	return result.Auctions, result.NextToken, nil
}
