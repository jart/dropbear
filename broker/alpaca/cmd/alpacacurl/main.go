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

type headers []string

func (h *headers) String() string { return strings.Join(*h, ", ") }
func (h *headers) Set(v string) error {
	*h = append(*h, v)
	return nil
}

func main() {
	var method string
	var hdrs headers
	var data string
	var jsonData string
	flag.StringVar(&method, "X", "", "HTTP method (GET, POST, PUT, PATCH, DELETE)")
	flag.Var(&hdrs, "H", "Header in 'Key: Value' format (repeatable)")
	flag.StringVar(&data, "d", "", "Request body")
	flag.StringVar(&jsonData, "json", "", "JSON request body (sets Content-Type: application/json)")
	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: alpacacurl [flags] URL")
		fmt.Fprintln(os.Stderr, "       alpacacurl /v2/orders")
		fmt.Fprintln(os.Stderr, "       alpacacurl -X POST -json '{\"qty\":\"1\"}' /v2/orders")
		fmt.Fprintln(os.Stderr, "       alpacacurl -X DELETE /v2/orders/ORDER_ID")
		fmt.Fprintln(os.Stderr, "       alpacacurl -H 'Accept: text/csv' /v2/stocks/AAPL/trades")
		fmt.Fprintln(os.Stderr, "       alpacacurl https://data.alpaca.markets/v2/...")
		os.Exit(1)
	}

	url := flag.Arg(0)
	if strings.HasPrefix(url, "/") {
		url = "https://api.alpaca.markets" + url
	}

	body := data
	if jsonData != "" {
		body = jsonData
	}

	if method == "" {
		if body != "" {
			method = "POST"
		} else {
			method = "GET"
		}
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	key, secret := alpaca.GetKey()
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("APCA-API-KEY-ID", key)
	req.Header.Set("APCA-API-SECRET-KEY", secret)

	if jsonData != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	for _, h := range hdrs {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			fmt.Fprintf(os.Stderr, "bad header (need 'Key: Value'): %s\n", h)
			os.Exit(1)
		}
		req.Header.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// Pretty print JSON
	var parsed any
	if json.Unmarshal(respBody, &parsed) == nil {
		out, _ := json.MarshalIndent(parsed, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Println(string(respBody))
	}

	if resp.StatusCode >= 400 {
		os.Exit(1)
	}
}
