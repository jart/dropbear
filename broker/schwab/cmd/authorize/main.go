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
	http.HandleFunc("/api/schwab", handleCallback)
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

	// exchange code for tokens (code expires in 30 seconds and is single-use)
	token, err := schwab.ExchangeCode(code)
	if err != nil {
		msg := fmt.Sprintf("token exchange failed: %v", err)
		fmt.Fprintln(w, "token exchange failed — click the authorize link again to get a fresh code (they expire in 30 seconds)")
		fmt.Println()
		fmt.Println(msg)
		fmt.Println()
		fmt.Println("the authorization code is single-use and expires in 30 seconds.")
		fmt.Println("click the authorize link again:")
		fmt.Println()
		fmt.Println("  " + schwab.AuthorizeURL())
		fmt.Println()
		return
	}

	fmt.Println("tokens saved successfully")
	fmt.Printf("  access token expires at:  %s\n", token.ExpiresAt)
	fmt.Printf("  refresh token expires at: %s\n", token.RefreshExpiresAt)
	fmt.Println()

	// discover account hash
	c := schwab.NewClient()
	_, err = c.SetAccount()
	if err != nil {
		// tokens are saved, this isn't fatal — user can retry
		msg := fmt.Sprintf("failed to discover account: %v", err)
		fmt.Fprintln(w, "tokens saved but account discovery failed — try running again")
		fmt.Println(msg)
		fmt.Println("tokens are saved. try running the command again to retry account discovery.")
		return
	}
	fmt.Println("authorization token saved to disk; you're good for 7 days")

	// write response
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("<h1>authorization successful</h1>\n"))

	// terminate program
	os.Exit(0)
}
