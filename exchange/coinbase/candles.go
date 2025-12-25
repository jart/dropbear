package coinbase

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/indicators"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

type CandleGranularity clocky.Duration

const (
	CandleGranularityMinute         = CandleGranularity(1 * clocky.Minute)
	CandleGranularityFiveMinutes    = CandleGranularity(5 * clocky.Minute)
	CandleGranularityFifteenMinutes = CandleGranularity(15 * clocky.Minute)
	CandleGranularityThirtyMinutes  = CandleGranularity(30 * clocky.Minute)
	CandleGranularityHour           = CandleGranularity(1 * clocky.Hour)
	CandleGranularityTwoHours       = CandleGranularity(2 * clocky.Hour)
	CandleGranularitySixHours       = CandleGranularity(6 * clocky.Hour)
	CandleGranularityDay            = CandleGranularity(1 * clocky.Day)
)

// GetCandles fetches historical candles for a product.
//
//   - productID is the Coinbase product identifier, e.g. BTC-USD.
//
//   - start is an inclusive UNIX second timestamp for the last candle
//     returned (most recent) which is rounded up to be divisible by 60.
//     Its default value is the time of the most recent candle available.
//
//   - end is an inclusive UNIX second timestamp for the first candle
//     returned (least recent) which is rounded down to be divisible by 60.
//     Its default value 350 minutes before start.
//
//   - limit is the maximum number of candles to return. Its default value
//     is 350. Its maximum value is 350.
//
// Returns candles in chronological order (oldest first).
func (c *Client) GetCandles(productID string, granularity CandleGranularity, start, end clocky.Time, limit int) ([]*indicators.Candle, error) {
	url := "/api/v3/brokerage/products/" + productID + "/candles?granularity=" + granularity.String()
	if start > 0 {
		url = fmt.Sprintf("%s&start=%d", url, start.Unix())
	}
	if end > 0 {
		url = fmt.Sprintf("%s&end=%d", url, end.Unix())
	}
	if limit > 0 {
		url = fmt.Sprintf("%s&limit=%d", url, limit)
	}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("candles request failed %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Candles []struct {
			Start  string `json:"start"`
			Low    string `json:"low"`
			High   string `json:"high"`
			Open   string `json:"open"`
			Close  string `json:"close"`
			Volume string `json:"volume"`
		} `json:"candles"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	n := len(result.Candles)
	candles := make([]*indicators.Candle, n)
	for i, rc := range result.Candles {
		start, _ := strconv.ParseInt(rc.Start, 10, 64)
		candles[n-1-i] = &indicators.Candle{
			Start:  clocky.Time(start * 1_000_000),
			Low:    decimal.Parse(rc.Low),
			High:   decimal.Parse(rc.High),
			Open:   decimal.Parse(rc.Open),
			Close:  decimal.Parse(rc.Close),
			Volume: decimal.Parse(rc.Volume),
		}
	}
	return candles, nil
}

func (cg CandleGranularity) String() string {
	switch cg {
	case CandleGranularityMinute:
		return "ONE_MINUTE"
	case CandleGranularityFiveMinutes:
		return "FIVE_MINUTE"
	case CandleGranularityFifteenMinutes:
		return "FIFTEEN_MINUTE"
	case CandleGranularityThirtyMinutes:
		return "THIRTY_MINUTE"
	case CandleGranularityHour:
		return "ONE_HOUR"
	case CandleGranularityTwoHours:
		return "TWO_HOUR"
	case CandleGranularitySixHours:
		return "SIX_HOUR"
	case CandleGranularityDay:
		return "ONE_DAY"
	default:
		panic("unknown candle granularity")
	}
}
