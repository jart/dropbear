// krakencurl makes authenticated Kraken API requests and prints raw JSON.
//
// Usage:
//
//	krakencurl /0/private/Balance
//	krakencurl /0/private/TradeVolume pair=XBTUSD,ETHUSD
//	krakencurl /0/public/AssetPairs pair=XBTUSD
package main

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"dropbear/broker/kraken"
	"dropbear/loggy"
)

func main() {
	loggy.Init()
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: krakencurl <path> [key=value ...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "examples:")
		fmt.Fprintln(os.Stderr, "  krakencurl /0/private/Balance")
		fmt.Fprintln(os.Stderr, "  krakencurl /0/private/TradeVolume pair=XBTUSD,ETHUSD")
		fmt.Fprintln(os.Stderr, "  krakencurl /0/public/AssetPairs pair=XBTUSD")
		os.Exit(1)
	}

	path := args[0]
	values := url.Values{}
	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			loggy.Fatalf("invalid argument %q, expected key=value", arg)
		}
		values.Set(parts[0], parts[1])
	}

	client := kraken.NewClient()
	defer client.Close()

	var body []byte
	if strings.Contains(path, "/public/") {
		resp, err := client.Get(path + "?" + values.Encode())
		if err != nil {
			loggy.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			loggy.Fatalf("reading response: %v", err)
		}
	} else {
		resp, err := client.Post(path, values)
		if err != nil {
			loggy.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			loggy.Fatalf("reading response: %v", err)
		}
	}

	fmt.Println(string(body))
}
