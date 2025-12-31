package coinbase

import (
	"dropbear/indicators"
	_ "embed"
	"fmt"
)

//go:embed daycandles.sql
var dayCandlesSchema string

// DayCandle represents a day candle from the database.
type DayCandle struct {
	Symbol string `json:"symbol"`
	Start  int64  `json:"start"`
	Open   string `json:"open"`
	High   string `json:"high"`
	Low    string `json:"low"`
	Close  string `json:"close"`
	Volume string `json:"volume"`
}

// SyncDayCandles fetches day candles from Coinbase API and stores them.
func (c *Client) SyncDayCandles(symbol string) (int, error) {
	productID := symbol + "-USD"
	candles, err := c.GetCandles(productID, CandleGranularityDay, 0, 0, 0)
	if err != nil {
		return 0, fmt.Errorf("fetching candles for %s: %w", symbol, err)
	}
	if err := c.upsertDayCandles(symbol, candles); err != nil {
		return 0, fmt.Errorf("upserting candles for %s: %w", symbol, err)
	}
	return len(candles), nil
}

// GetDayCandles retrieves all day candles for a symbol, ordered by date.
func (c *Client) GetDayCandles(symbol string) ([]DayCandle, error) {
	if err := c.initDayCandles(); err != nil {
		return nil, err
	}
	rows, err := c.db.Query(`
		SELECT symbol, start, open, high, low, close, volume
		FROM coinbase_daycandles
		WHERE symbol = ?
		ORDER BY start ASC
	`, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candles []DayCandle
	for rows.Next() {
		var c DayCandle
		if err := rows.Scan(&c.Symbol, &c.Start, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, err
		}
		candles = append(candles, c)
	}
	return candles, rows.Err()
}

// GetAllDayCandles retrieves all day candles for all symbols, ordered by symbol and date.
func (c *Client) GetAllDayCandles() (map[string][]DayCandle, error) {
	if err := c.initDayCandles(); err != nil {
		return nil, err
	}
	rows, err := c.db.Query(`
		SELECT symbol, start, open, high, low, close, volume
		FROM coinbase_daycandles
		ORDER BY symbol ASC, start ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]DayCandle)
	for rows.Next() {
		var c DayCandle
		if err := rows.Scan(&c.Symbol, &c.Start, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume); err != nil {
			return nil, err
		}
		result[c.Symbol] = append(result[c.Symbol], c)
	}
	return result, rows.Err()
}

func (c *Client) initDayCandles() error {
	c.dayCandlesOnce.Do(func() {
		if _, err := c.db.Exec(dayCandlesSchema); err != nil {
			c.dayCandlesErr = err
			return
		}
		c.dayCandlesUpsert, c.dayCandlesErr = c.db.Prepare(`
			INSERT OR REPLACE INTO coinbase_daycandles (
				symbol, start, open, high, low, close, volume
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`)
	})
	return c.dayCandlesErr
}

// UpsertDayCandle inserts or updates a single day candle.
func (c *Client) upsertDayCandle(symbol string, candle *indicators.Candle) error {
	if err := c.initDayCandles(); err != nil {
		return err
	}
	_, err := c.dayCandlesUpsert.Exec(
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

// upsertDayCandles inserts or updates multiple day candles in a transaction.
func (c *Client) upsertDayCandles(symbol string, candles []*indicators.Candle) error {
	if err := c.initDayCandles(); err != nil {
		return err
	}
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt := tx.Stmt(c.dayCandlesUpsert)
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
