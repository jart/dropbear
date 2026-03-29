package main

import (
	"database/sql"
	_ "embed"
)

//go:embed schema.sql
var schemaDDL string

func Migrate(db *sql.DB) error {
	_, err := db.Exec(schemaDDL)
	return err
}
