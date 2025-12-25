package db

import (
	"database/sql"
	"dropbear/loggy"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var (
	flagDB = flag.String("db", "~/.dropbear.sqlite3", "Path to the database for coinbase fills")
	dbOnce sync.Once
	dbSave *sql.DB
)

// Get returns memoized database connection.
func Get() *sql.DB {
	dbOnce.Do(func() { dbSave = MustOpen(*flagDB) })
	return dbSave
}

// MustOpen opens a database connection or fatally logs.
func MustOpen(path string) *sql.DB {
	db, err := Open(path)
	if err != nil {
		loggy.Fatalf("error opening database %s: %v", path, err)
	}
	return db
}

// Open opens a database connection.
func Open(path string) (*sql.DB, error) {
	path = expandPath(path)
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	// this is needed to avoid "database is locked" errors
	if _, err = conn.Exec("PRAGMA busy_timeout=1000"); err != nil {
		return nil, err
	}
	// wal2 is fast like wal but ensures readers never block writers
	if _, err = conn.Exec("PRAGMA journal_mode=wal2"); err != nil {
		return nil, err
	}
	// we're doing high-frequency trading, it's not like we're a bank
	if _, err = conn.Exec("PRAGMA synchronous=OFF"); err != nil {
		return nil, err
	}
	return conn, nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
