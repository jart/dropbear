package alpaca

import (
	"dropbear/clocky"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"dropbear/netty"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrBarsUnsupportedLimit    = errors.New("unsupported limit for bars")
	ErrBarsUnsupportedDuration = errors.New("unsupported duration for bars")
)

// GetBars fetches historical OHLC data for a stock symbol.
// Returns bars and the next page token (empty if no more pages).
// start defaults to the beginning of the current day, but at least 15 minutes ago if the user doesn't have real-time access for the feed.
// end default to the current time if the user has a real-time access for the feed, otherwise 15 minutes before the current time.
// limit defaults to 1000 if 0 is passed and the maximum is 10000.
// bars are returned by this api in ascending order.
// https://docs.alpaca.markets/reference/stockbars
func (c *Client) GetBars(sym symbol.Symbol, timeframe clocky.Duration, start, end clocky.Time, feed DataFeed, adjustment BarAdjustment, limit int, desc bool, pageToken string) ([]ds.Bar, string, error) {
	if limit < 0 || limit > 10000 {
		return nil, "", ErrBarsUnsupportedLimit
	}
	tf := convertDurationToTimeframe(timeframe)
	if tf == "" {
		return nil, "", ErrBarsUnsupportedDuration
	}
	c.DataTokenBucket.Get()
	u := &url.URL{
		Scheme: "https",
		Host:   DataHost,
		Path:   "/v2/stocks/" + sym.String() + "/bars",
	}
	q := u.Query()
	q.Set("timeframe", tf)
	if start != 0 {
		q.Set("start", start.RFC3339())
	}
	if end != 0 {
		q.Set("end", end.RFC3339())
	}
	if feed != 0 {
		q.Set("feed", feed.String())
	}
	if adjustment != 0 {
		q.Set("adjustment", adjustment.String())
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
		Bars      []ds.Bar `json:"bars"`
		NextToken string   `json:"next_page_token"`
	}
	err := c.RequestJSON(netty.BulkHttpClient, "GET", u.String(), nil, &result)
	if err != nil {
		return nil, "", err
	}
	return result.Bars, result.NextToken, nil
}

// GetBarsForSymbols fetches historical OHLC data for multiple stock symbols.
// Returns bars and the next page token (empty if no more pages).
// start defaults to the beginning of the current day, but at least 15 minutes ago if the user doesn't have real-time access for the feed.
// end default to the current time if the user has a real-time access for the feed, otherwise 15 minutes before the current time.
// limit defaults to 1000 if 0 is passed and the maximum is 10000.
// bars are returned by this api in ascending order.
// https://docs.alpaca.markets/reference/stockbars
func (c *Client) GetBarsForSymbols(symbols []symbol.Symbol, timeframe clocky.Duration, start, end clocky.Time, feed DataFeed, adjustment BarAdjustment, limit int, desc bool, pageToken string) (map[symbol.Symbol][]ds.Bar, string, error) {
	if len(symbols) == 0 {
		return nil, "", nil
	}
	if limit < 0 || limit > 10000 {
		return nil, "", ErrBarsUnsupportedLimit
	}
	tf := convertDurationToTimeframe(timeframe)
	if tf == "" {
		return nil, "", ErrBarsUnsupportedDuration
	}
	c.DataTokenBucket.Get()
	u := &url.URL{
		Scheme: "https",
		Host:   DataHost,
		Path:   "/v2/stocks/bars",
	}
	q := u.Query()
	var sb strings.Builder
	for i, sym := range symbols {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(sym.String())
	}
	q.Set("symbols", sb.String())
	q.Set("timeframe", tf)
	if start != 0 {
		q.Set("start", start.RFC3339())
	}
	if end != 0 {
		q.Set("end", end.RFC3339())
	}
	if feed != 0 {
		q.Set("feed", feed.String())
	}
	if adjustment != 0 {
		q.Set("adjustment", adjustment.String())
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
		Bars      map[symbol.Symbol][]ds.Bar `json:"bars"`
		NextToken string                     `json:"next_page_token"`
	}
	err := c.RequestJSON(netty.BulkHttpClient, "GET", u.String(), nil, &result)
	if err != nil {
		return nil, "", err
	}
	return result.Bars, result.NextToken, nil
}

func convertDurationToTimeframe(d clocky.Duration) string {
	i := int64(d)
	if i <= 0 {
		return ""
	}
	m := i / int64(clocky.Minute)
	if i%int64(clocky.Minute) != 0 {
		return ""
	}
	if m <= 9 {
		var b [2]byte
		b[0] = '0' + byte(m)
		b[1] = 'T'
		return string(b[:])
	}
	if m <= 59 {
		var b [3]byte
		b[0] = '0' + byte(m/10)
		b[1] = '0' + byte(m%10)
		b[2] = 'T'
		return string(b[:])
	}
	h := m / 60
	if m%60 != 0 {
		return ""
	}
	if h <= 9 {
		var b [2]byte
		b[0] = '0' + byte(h)
		b[1] = 'H'
		return string(b[:])
	}
	if h <= 23 {
		var b [3]byte
		b[0] = '0' + byte(h/10)
		b[1] = '0' + byte(h%10)
		b[2] = 'H'
		return string(b[:])
	}
	switch d {
	case clocky.Day:
		return "1D"
	case clocky.Week:
		return "1W"
	case clocky.Monthy:
		return "1M"
	case clocky.Monthy * 2:
		return "2M"
	case clocky.Monthy * 3:
		return "3M"
	case clocky.Monthy * 4:
		return "4M"
	case clocky.Monthy * 6:
		return "6M"
	case clocky.Monthy * 12:
		return "12M"
	default:
		return ""
	}
}
