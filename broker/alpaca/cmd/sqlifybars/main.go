// sqlifybars imports equity bar data from binary files into SQLite.
//
// Usage:
//
//	go run ./broker/alpaca/cmd/sqlifybars GOOG
//	go run ./broker/alpaca/cmd/sqlifybars GOOG AMZN META
//
// This reads ~/equitydata/minutes/{SYMBOL} and inserts the bars into the
// equity_bars table in ~/.dropbear.sqlite3.
package main

import (
	"flag"
	"log"
	"path/filepath"

	"dropbear/db"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"dropbear/loggy"
)

func main() {
	flag.Parse()
	loggy.Init()

	if len(flag.Args()) == 0 {
		log.Fatal("usage: sqlifybars SYMBOL [SYMBOL...]")
	}

	conn := db.Get()

	// Drop and recreate table for clean import
	_, _ = conn.Exec(`DROP TABLE IF EXISTS equity_bars`)
	_, err := conn.Exec(`
		CREATE TABLE equity_bars (
			symbol TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			open REAL NOT NULL,
			high REAL NOT NULL,
			low REAL NOT NULL,
			close REAL NOT NULL,
			volume REAL NOT NULL,
			trade_count INTEGER NOT NULL,
			vwap REAL NOT NULL,
			PRIMARY KEY (symbol, timestamp)
		)
	`)
	if err != nil {
		log.Fatalf("failed to create table: %v", err)
	}

	// Create index on timestamp for range queries
	_, err = conn.Exec(`
		CREATE INDEX IF NOT EXISTS idx_equity_bars_timestamp
		ON equity_bars (timestamp)
	`)
	if err != nil {
		log.Fatalf("failed to create index: %v", err)
	}

	for _, arg := range flag.Args() {
		sym, err := symbol.Parse(arg)
		if err != nil {
			log.Printf("invalid symbol %q: %v", arg, err)
			continue
		}
		importSymbol(sym)
	}

	log.Println("done")
}

func importSymbol(sym symbol.Symbol) {
	path := filepath.Join(ds.EquityMinutesDir(), sym.String())
	bars, err := ds.OpenBars(path)
	if err != nil {
		log.Printf("%s: failed to open bars: %v", sym, err)
		return
	}
	defer bars.Close()

	conn := db.Get()

	// Use a transaction for speed
	tx, err := conn.Begin()
	if err != nil {
		log.Printf("%s: failed to begin transaction: %v", sym, err)
		return
	}

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO equity_bars
		(symbol, timestamp, open, high, low, close, volume, trade_count, vwap)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		log.Printf("%s: failed to prepare statement: %v", sym, err)
		return
	}
	defer stmt.Close()

	symStr := sym.String()
	count := 0
	for !bars.EOF() {
		bar := bars.Read()
		_, err := stmt.Exec(
			symStr,
			int64(bar.Timestamp),
			bar.Open.Float64(),
			bar.High.Float64(),
			bar.Low.Float64(),
			bar.Close.Float64(),
			bar.Volume.Float64(),
			bar.TradeCount,
			bar.VWAP.Float64(),
		)
		if err != nil {
			tx.Rollback()
			log.Printf("%s: failed to insert bar: %v", sym, err)
			return
		}
		count++
		if count%100000 == 0 {
			log.Printf("%s: imported %d bars...", sym, count)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("%s: failed to commit: %v", sym, err)
		return
	}

	log.Printf("%s: imported %d bars", sym, count)
}
