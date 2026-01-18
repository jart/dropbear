package main

import (
	"dropbear/broker/coinbase"
	"dropbear/db"
	"dropbear/loggy"
	"flag"
	"fmt"
	"log"
	"os"
)

func sync(dbPath string, coinbaseKeyPath string) {
	db, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not open database %s: %v", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()
	key, err := coinbase.LoadKey(coinbaseKeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not load coinbase key %s: %v", coinbaseKeyPath, err)
		os.Exit(1)
	}
	client := coinbase.NewClientWith(key, db)
	defer client.Close()
	accounts, err := client.GetAccounts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not load coinbase accounts: %v", err)
		os.Exit(1)
	}
	for _, account := range accounts {
		log.Printf("%s has account %s id %s", coinbaseKeyPath, account.Currency, account.UUID)
		if err := client.SyncTransactions(account.Currency); err != nil {
			fmt.Fprintf(os.Stderr, "could not sync coinbase transactions for %s: %v", account.Currency, err)
			os.Exit(1)
		}
	}
}

func main() {
	flag.Parse()
	loggy.Init()
	sync(os.ExpandEnv("$HOME/.primary.sqlite3"), os.ExpandEnv("$HOME/.primary.key"))
	sync(os.ExpandEnv("$HOME/.dropbear.sqlite3"), os.ExpandEnv("$HOME/.coinbase.key"))
	sync(os.ExpandEnv("$HOME/.zec.sqlite3"), os.ExpandEnv("$HOME/.zec.key"))
}
