package coinbase

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/indicators"
	_ "embed"
	"fmt"
	"slices"
)

//go:embed minutecandles.sql
var minuteCandlesSchema string

const minuteMicros = 60 * 1_000_000

// MinuteCandle represents a minute candle from the database.
type MinuteCandle struct {
	Symbol string          `json:"symbol"`
	Start  clocky.Time     `json:"start"`
	Open   decimal.Decimal `json:"open"`
	High   decimal.Decimal `json:"high"`
	Low    decimal.Decimal `json:"low"`
	Close  decimal.Decimal `json:"close"`
	Volume decimal.Decimal `json:"volume"`
}

// GetMinuteCandlesFromDB retrieves minute candles for a symbol in a time range from the database.
// startMicros and endMicros are inclusive unix microsecond timestamps.
func (c *Client) GetMinuteCandlesFromDB(symbol string, startMicros, endMicros clocky.Time) ([]MinuteCandle, error) {
	if err := c.initMinuteCandles(); err != nil {
		return nil, err
	}
	rows, err := c.db.Query(`
		SELECT symbol, start, open, high, low, close, volume
		FROM coinbase_minutecandles
		WHERE symbol = ? AND start >= ? AND start <= ?
		ORDER BY start ASC
	`, symbol, startMicros, endMicros)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candles []MinuteCandle
	for rows.Next() {
		var c MinuteCandle
		var open, high, low, close, volume string
		if err := rows.Scan(&c.Symbol, &c.Start, &open, &high, &low, &close, &volume); err != nil {
			return nil, err
		}
		c.Open = decimal.Parse(open)
		c.High = decimal.Parse(high)
		c.Low = decimal.Parse(low)
		c.Close = decimal.Parse(close)
		c.Volume = decimal.Parse(volume)
		candles = append(candles, c)
	}
	return candles, rows.Err()
}

// GetMinuteCandlesRange retrieves minute candles for a symbol in a time range,
// fetching from API and caching any missing data.
// startMicros and endMicros are inclusive unix microsecond timestamps.
// Returns candles in chronological order (oldest first).
func (c *Client) GetMinuteCandlesRange(symbol string, startMicros, endMicros clocky.Time) ([]MinuteCandle, error) {
	// Align to minute boundaries
	startMicros = (startMicros / minuteMicros) * minuteMicros
	endMicros = (endMicros / minuteMicros) * minuteMicros

	// Get what we have in the database
	cached, err := c.GetMinuteCandlesFromDB(symbol, startMicros, endMicros)
	if err != nil {
		return nil, fmt.Errorf("querying cached candles: %w", err)
	}

	// Build set of timestamps we have
	have := make(map[clocky.Time]bool)
	for _, candle := range cached {
		have[candle.Start] = true
	}

	// Find gaps (missing minutes)
	var gaps []clocky.Time
	for ts := startMicros; ts <= endMicros; ts += minuteMicros {
		if !have[ts] {
			gaps = append(gaps, ts)
		}
	}

	if len(gaps) == 0 {
		return cached, nil
	}

	// Fetch missing ranges from API
	// Group gaps into contiguous ranges to minimize API calls
	ranges := groupIntoRanges(gaps)
	productID := symbol + "-USD"

	// API limits to 350 candles per request
	const maxCandlesPerRequest = 300 // Use 300 to have some buffer
	const maxRangeMicros = maxCandlesPerRequest * minuteMicros

	for _, r := range ranges {
		// Break up large ranges into smaller chunks
		for chunkStart := r.start; chunkStart <= r.end; chunkStart += maxRangeMicros {
			chunkEnd := min(chunkStart+maxRangeMicros-minuteMicros, r.end)
			candles, err := c.GetCandles(productID, CandleGranularityMinute, chunkStart, chunkEnd, 0)
			if err != nil {
				return nil, fmt.Errorf("fetching candles for %s [%s-%s]: %w", symbol, chunkStart, chunkEnd, err)
			}
			if err := c.upsertMinuteCandles(symbol, candles); err != nil {
				return nil, fmt.Errorf("caching candles for %s: %w", symbol, err)
			}
		}
	}

	// Re-fetch from database to get the complete set
	return c.GetMinuteCandlesFromDB(symbol, startMicros, endMicros)
}

// timeRange represents a contiguous range of timestamps.
type timeRange struct {
	start, end clocky.Time
}

// groupIntoRanges groups a sorted list of timestamps into contiguous ranges.
func groupIntoRanges(timestamps []clocky.Time) []timeRange {
	if len(timestamps) == 0 {
		return nil
	}
	slices.Sort(timestamps)
	var ranges []timeRange
	start := timestamps[0]
	end := timestamps[0]
	for i := 1; i < len(timestamps); i++ {
		if timestamps[i] == end+minuteMicros {
			// Contiguous
			end = timestamps[i]
		} else {
			// Gap - save current range and start new one
			ranges = append(ranges, timeRange{start, end})
			start = timestamps[i]
			end = timestamps[i]
		}
	}
	ranges = append(ranges, timeRange{start, end})
	return ranges
}

// GetMinuteCandleAt retrieves the minute candle at or before a specific timestamp.
// Useful for getting the price at a specific point in time.
func (c *Client) GetMinuteCandleAt(symbol string, micros clocky.Time) (*MinuteCandle, error) {
	// Align to minute boundary
	aligned := (micros / minuteMicros) * minuteMicros

	// Try to get from cache first
	candles, err := c.GetMinuteCandlesFromDB(symbol, aligned, aligned)
	if err != nil {
		return nil, err
	}
	if len(candles) > 0 {
		return &candles[0], nil
	}

	// Fetch from API - get a small range around the target
	rangeStart := aligned - 5*minuteMicros
	rangeEnd := aligned + 5*minuteMicros
	candles, err = c.GetMinuteCandlesRange(symbol, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}

	// Find the candle at or before our target time
	for i := len(candles) - 1; i >= 0; i-- {
		if clocky.Time(candles[i].Start) <= aligned {
			return &candles[i], nil
		}
	}

	if len(candles) > 0 {
		return &candles[0], nil
	}

	return nil, fmt.Errorf("no candle found for %s at %d", symbol, micros)
}

func (c *Client) initMinuteCandles() error {
	c.minuteCandlesOnce.Do(func() {
		if _, err := c.db.Exec(minuteCandlesSchema); err != nil {
			c.minuteCandlesErr = err
			return
		}
		c.minuteCandlesUpsert, c.minuteCandlesErr = c.db.Prepare(`
			INSERT OR REPLACE INTO coinbase_minutecandles (
				symbol, start, open, high, low, close, volume
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`)
	})
	return c.minuteCandlesErr
}

// UpsertMinuteCandle inserts or updates a single minute candle.
func (c *Client) UpsertMinuteCandle(symbol string, candle *indicators.Candle) error {
	if err := c.initMinuteCandles(); err != nil {
		return err
	}
	_, err := c.minuteCandlesUpsert.Exec(
		symbol,
		candle.Start,
		candle.Open.String(),
		candle.High.String(),
		candle.Low.String(),
		candle.Close.String(),
		candle.Volume.String(),
	)
	return err
}

// UpsertMinuteCandles inserts or updates multiple minute candles in a transaction.
func (c *Client) upsertMinuteCandles(symbol string, candles []*indicators.Candle) error {
	if err := c.initMinuteCandles(); err != nil {
		return err
	}
	if len(candles) == 0 {
		return nil
	}
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt := tx.Stmt(c.minuteCandlesUpsert)
	for _, candle := range candles {
		_, err := stmt.Exec(
			symbol,
			candle.Start,
			candle.Open.String(),
			candle.High.String(),
			candle.Low.String(),
			candle.Close.String(),
			candle.Volume.String(),
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
