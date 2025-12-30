package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"dropbear/broker/alpaca"
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: alpacacurl URL")
		fmt.Fprintln(os.Stderr, "       alpacacurl /v2/stocks/AAPL/trades?start=...")
		fmt.Fprintln(os.Stderr, "       alpacacurl https://data.alpaca.markets/v2/...")
		os.Exit(1)
	}

	url := flag.Arg(0)
	if strings.HasPrefix(url, "/") {
		url = "https://data.alpaca.markets" + url
	}

	key, secret := alpaca.GetKey()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("APCA-API-KEY-ID", key)
	req.Header.Set("APCA-API-SECRET-KEY", secret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Pretty print JSON
	var data any
	if json.Unmarshal(body, &data) == nil {
		out, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Println(string(body))
	}

	if resp.StatusCode != 200 {
		os.Exit(1)
	}
}
