// Command schwabcurl makes authenticated Schwab API requests and prints the JSON response.
//
// Usage:
//
//	go run ./broker/schwab/cmd/schwabcurl /accounts/accountNumbers
//	go run ./broker/schwab/cmd/schwabcurl /accounts
//	go run ./broker/schwab/cmd/schwabcurl /accounts/{hash}/orders?fromEnteredTime=2024-01-01T00:00:00.000Z&toEnteredTime=2024-12-31T23:59:59.000Z
package main

import (
	"dropbear/broker/schwab"
	"dropbear/netty"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: schwabcurl <path>\n")
		fmt.Fprintf(os.Stderr, "  e.g. schwabcurl /accounts/accountNumbers\n")
		os.Exit(1)
	}
	path := flag.Arg(0)
	c := schwab.NewClient()
	resp, err := c.Request(netty.BulkHttpClient, "GET", path, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s\n", body)
	if resp.StatusCode >= 400 {
		os.Exit(1)
	}
}
