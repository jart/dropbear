// Command import.equity imports equity minute candle data into SQLite.
//
// Usage:
//
//	go run ./cmd/import.equity
package main

import (
	"database/sql"
	"dropbear/clocky"
	"dropbear/db"
	"dropbear/indicators"
	"dropbear/loggy"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
)

var (
	flagDir = flag.String("dir", os.ExpandEnv("$HOME/equitydata/minutes"), "directory containing equity data")
)

func main() {
	flag.Parse()
	loggy.Init()

	conn := db.Get()
	createSchema(conn)

	// List all symbols
	entries, err := os.ReadDir(*flagDir)
	if err != nil {
		loggy.Fatalf("failed to read directory: %v", err)
	}

	var symbols []string
	for _, entry := range entries {
		if entry.IsDir() {
			symbols = append(symbols, entry.Name())
		}
	}

	log.Printf("Found %d symbols to import", len(symbols))

	// Process symbols sequentially (SQLite doesn't like parallel writes)
	var totalCandles int64
	for i, symbol := range symbols {
		n := importSymbol(conn, symbol)
		totalCandles += int64(n)
		if (i+1)%10 == 0 {
			log.Printf("Imported %d/%d symbols (%d candles)", i+1, len(symbols), totalCandles)
		}
	}

	log.Printf("Done! Imported %d symbols, %d candles", len(symbols), totalCandles)

	// Create summary view
	createSummary(conn)
}

func createSchema(conn *sql.DB) {
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS equity_candles (
			symbol TEXT NOT NULL,
			start INTEGER NOT NULL,
			open INTEGER NOT NULL,
			high INTEGER NOT NULL,
			low INTEGER NOT NULL,
			close INTEGER NOT NULL,
			volume INTEGER NOT NULL,
			PRIMARY KEY (symbol, start)
		) WITHOUT ROWID;

		CREATE INDEX IF NOT EXISTS equity_candles_start ON equity_candles(start);
	`)
	if err != nil {
		loggy.Fatalf("failed to create schema: %v", err)
	}
}

func importSymbol(conn *sql.DB, symbol string) int {
	symbolDir := filepath.Join(*flagDir, symbol)
	entries, err := os.ReadDir(symbolDir)
	if err != nil {
		log.Printf("Warning: failed to read %s: %v", symbolDir, err)
		return 0
	}

	tx, err := conn.Begin()
	if err != nil {
		loggy.Fatalf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO equity_candles
		(symbol, start, open, high, low, close, volume) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		loggy.Fatalf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	var count int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(symbolDir, entry.Name())
		n := importFile(stmt, symbol, path)
		count += n
	}

	if err := tx.Commit(); err != nil {
		loggy.Fatalf("failed to commit: %v", err)
	}
	return count
}

func importFile(stmt *sql.Stmt, symbol, path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	reader, err := zstd.NewReader(file)
	if err != nil {
		return 0
	}
	defer reader.Close()

	var candle indicators.Candle
	var count int
	for {
		err := candle.Deserialize(reader)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			log.Printf("Warning: error reading %s: %v", path, err)
			break
		}

		_, err = stmt.Exec(
			symbol,
			int64(candle.Start),
			int64(candle.Open),
			int64(candle.High),
			int64(candle.Low),
			int64(candle.Close),
			int64(candle.Volume),
		)
		if err != nil {
			log.Printf("Warning: failed to insert candle: %v", err)
			continue
		}
		count++
	}
	return count
}

func createSummary(conn *sql.DB) {
	log.Printf("Creating summary view...")
	_, err := conn.Exec(`
		DROP VIEW IF EXISTS equity_summary;
		CREATE VIEW equity_summary AS
		SELECT
			symbol,
			COUNT(*) as candle_count,
			datetime(MIN(start)/1000000, 'unixepoch') as first_candle,
			datetime(MAX(start)/1000000, 'unixepoch') as last_candle,
			CAST((MAX(start) - MIN(start)) / 1000000 / 86400 AS INT) as days_of_data
		FROM equity_candles
		GROUP BY symbol
		ORDER BY first_candle, symbol;
	`)
	if err != nil {
		log.Printf("Warning: failed to create summary view: %v", err)
	}

	// Show symbols with full data
	rows, err := conn.Query(`
		SELECT symbol, first_candle, last_candle, days_of_data
		FROM equity_summary
		WHERE first_candle < '2016-02-01'
		ORDER BY symbol
		LIMIT 20
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	fmt.Println("\nSymbols with data from 2016:")
	for rows.Next() {
		var symbol, first, last string
		var days int
		rows.Scan(&symbol, &first, &last, &days)
		fmt.Printf("  %s: %s to %s (%d days)\n", symbol, first[:10], last[:10], days)
	}

	// Count symbols with full data
	var fullCount int
	conn.QueryRow(`SELECT COUNT(*) FROM equity_summary WHERE first_candle < '2016-02-01'`).Scan(&fullCount)
	fmt.Printf("\nTotal symbols with 2016+ data: %d\n", fullCount)

	// Show date range info
	var minTime, maxTime clocky.Time
	conn.QueryRow(`SELECT MIN(start), MAX(start) FROM equity_candles`).Scan(&minTime, &maxTime)
	fmt.Printf("Overall data range: %s to %s\n", minTime.String()[:10], maxTime.String()[:10])
}
