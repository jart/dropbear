// Command authorize performs the Schwab OAuth authorization flow.
//
// Run this once every 7 days to get fresh tokens. It starts a local HTTP
// server, prints the Schwab authorize URL for you to visit in your browser,
// waits for the OAuth callback (proxied through nginx), exchanges the code
// for tokens, and verifies everything works by calling GetAccounts.
//
// nginx config needed:
//
//	location = /api/schwab {
//	    proxy_pass http://127.0.0.1:8420;
//	}
//
// Usage:
//
//	go run ./broker/schwab/cmd/authorize
package main

import (
	"dropbear/broker/schwab"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
)

var (
	flagAddr = flag.String("addr", "127.0.0.1:8420", "local address to listen on for OAuth callback")
)

func main() {
	flag.Parse()
	fmt.Println()
	fmt.Println("visit this url to authorize:")
	fmt.Println()
	fmt.Println("  " + schwab.AuthorizeURL())
	fmt.Println()
	fmt.Println("waiting for callback on " + *flagAddr + " ...")
	fmt.Println()
	http.HandleFunc("/", handleCallback)
	if err := http.ListenAndServe(*flagAddr, nil); err != nil {
		log.Fatal(err)
	}
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		return
	}

	// URL-decode the code (must end in '@' not '%40')
	code, err := url.QueryUnescape(code)
	if err != nil {
		http.Error(w, "failed to decode code: "+err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Println("received authorization code, exchanging for tokens...")

	token, err := schwab.ExchangeCode(code)
	if err != nil {
		msg := fmt.Sprintf("token exchange failed: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		log.Fatal(msg)
	}

	fmt.Println("tokens saved successfully")
	fmt.Printf("  access token expires at:  %s\n", token.ExpiresAt)
	fmt.Printf("  refresh token expires at: %s\n", token.RefreshExpiresAt)
	fmt.Println()

	// discover account hash
	c := schwab.NewClient()
	hash, err := c.SetAccount()
	if err != nil {
		msg := fmt.Sprintf("failed to discover account: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		log.Fatal(msg)
	}
	fmt.Printf("account hash: %s\n", hash)

	// hello world: fetch accounts
	accounts, err := c.GetAccounts()
	if err != nil {
		msg := fmt.Sprintf("GetAccounts failed: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		log.Fatal(msg)
	}
	fmt.Printf("found %d account(s)\n", len(accounts))
	for _, a := range accounts {
		acct := a.SecuritiesAccount
		fmt.Printf("  account %s: equity=%s buying_power=%s\n",
			acct.AccountNumber,
			acct.CurrentBalances.Equity,
			acct.CurrentBalances.BuyingPower)
	}

	fmt.Fprintln(w, "authorization successful, you can close this tab")
	fmt.Println()
	fmt.Println("done! you're good for 7 days.")

	// exit cleanly after responding
	go func() { os.Exit(0) }()
}
