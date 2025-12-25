package coinbase

import (
	"database/sql"
	"dropbear/db"
	"dropbear/indicators"
	_ "embed"
	"fmt"
	"sync"
)

//go:embed daycandles.sql
var dayCandlesSchema string

var (
	dayCandlesOnce   sync.Once
	dayCandlesUpsert *sql.Stmt
	dayCandlesErr    error
)

func initDayCandles() error {
	dayCandlesOnce.Do(func() {
		database := db.Get()
		if _, err := database.Exec(dayCandlesSchema); err != nil {
			dayCandlesErr = err
			return
		}
		dayCandlesUpsert, dayCandlesErr = database.Prepare(`
			INSERT OR REPLACE INTO coinbase_daycandles (
				symbol, start, open, high, low, close, volume
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`)
	})
	return dayCandlesErr
}

// UpsertDayCandle inserts or updates a single day candle.
func UpsertDayCandle(symbol string, c *indicators.Candle) error {
	if err := initDayCandles(); err != nil {
		return err
	}
	_, err := dayCandlesUpsert.Exec(
		symbol,
		c.Start,
		c.Open.String(),
		c.High.String(),
		c.Low.String(),
		c.Close.String(),
		c.Volume.String(),
	)
	return err
}

// UpsertDayCandles inserts or updates multiple day candles in a transaction.
func UpsertDayCandles(symbol string, candles []*indicators.Candle) error {
	if err := initDayCandles(); err != nil {
		return err
	}
	database := db.Get()
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt := tx.Stmt(dayCandlesUpsert)
	for _, c := range candles {
		_, err := stmt.Exec(
			symbol,
			c.Start,
			c.Open.String(),
			c.High.String(),
			c.Low.String(),
			c.Close.String(),
			c.Volume.String(),
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

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

// GetDayCandles retrieves all day candles for a symbol, ordered by date.
func GetDayCandles(symbol string) ([]DayCandle, error) {
	if err := initDayCandles(); err != nil {
		return nil, err
	}
	database := db.Get()
	rows, err := database.Query(`
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
func GetAllDayCandles() (map[string][]DayCandle, error) {
	if err := initDayCandles(); err != nil {
		return nil, err
	}
	database := db.Get()
	rows, err := database.Query(`
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

// SyncDayCandles fetches day candles from Coinbase API and stores them.
func (c *Client) SyncDayCandles(symbol string) (int, error) {
	productID := symbol + "-USD"
	candles, err := c.GetCandles(productID, CandleGranularityDay, 0, 0, 0)
	if err != nil {
		return 0, fmt.Errorf("fetching candles for %s: %w", symbol, err)
	}
	if err := UpsertDayCandles(symbol, candles); err != nil {
		return 0, fmt.Errorf("upserting candles for %s: %w", symbol, err)
	}
	return len(candles), nil
}
