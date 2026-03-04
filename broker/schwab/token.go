package schwab

import (
	"dropbear/clocky"
	"dropbear/netty"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

const (
	oauthBaseURL = "https://api.schwabapi.com/v1/oauth"
)

var (
	flagToken = flag.String("schwab-token", os.ExpandEnv("$HOME/.schwab.token"), "path to schwab token json file")
)

// Token holds the OAuth2 access and refresh tokens.
type Token struct {
	AccessToken      string      `json:"access_token"`
	RefreshToken     string      `json:"refresh_token"`
	ExpiresAt        clocky.Time `json:"expires_at"`         // when access token expires
	RefreshExpiresAt clocky.Time `json:"refresh_expires_at"` // when refresh token expires
	AccountHash      string      `json:"account_hash,omitempty"`
	AccountNumber    string      `json:"account_number,omitempty"`
}

var (
	tokenMu   sync.Mutex
	tokenSave *Token
)

// loadToken reads the token from disk, or returns nil if not found.
func loadToken() *Token {
	data, err := os.ReadFile(*flagToken)
	if err != nil {
		return nil
	}
	var t Token
	if err := json.Unmarshal(data, &t); err != nil {
		log.Printf("schwab: failed to parse token file: %v", err)
		return nil
	}
	return &t
}

// saveToken persists the token to disk.
func saveToken(t *Token) {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		log.Printf("schwab: failed to marshal token: %v", err)
		return
	}
	if err := os.WriteFile(*flagToken, append(data, '\n'), 0600); err != nil {
		log.Printf("schwab: failed to write token file: %v", err)
	}
}

// getToken returns the cached token, loading from disk if needed.
func getToken() *Token {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	if tokenSave == nil {
		tokenSave = loadToken()
	}
	return tokenSave
}

// setToken updates the cached token and persists to disk.
func setToken(t *Token) {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	tokenSave = t
	saveToken(t)
}

// AuthorizeURL returns the URL the user must visit to authorize the app.
func AuthorizeURL() string {
	key := GetKey()
	return fmt.Sprintf("%s/authorize?client_id=%s&redirect_uri=%s",
		oauthBaseURL,
		url.QueryEscape(key.Key),
		url.QueryEscape(key.Callback))
}

// basicAuth returns the Base64-encoded Client_ID:Client_Secret for OAuth requests.
func basicAuth() string {
	key := GetKey()
	return base64.StdEncoding.EncodeToString([]byte(key.Key + ":" + key.Secret))
}

// tokenResponse is the raw JSON response from the OAuth token endpoint.
type tokenResponse struct {
	ExpiresIn    int64  `json:"expires_in"`    // seconds until access token expires
	TokenType    string `json:"token_type"`    // "Bearer"
	Scope        string `json:"scope"`         // "api"
	RefreshToken string `json:"refresh_token"` // valid 7 days
	AccessToken  string `json:"access_token"`  // valid 30 minutes
	IDToken      string `json:"id_token"`      // JWT
}

// ExchangeCode exchanges an authorization code for access and refresh tokens.
// The code must be URL-decoded before calling this function (e.g. ending in '@' not '%40').
func ExchangeCode(code string) (*Token, error) {
	key := GetKey()
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", key.Callback)
	return postToken(form)
}

// refreshAccessToken uses the refresh token to obtain a new access token.
func refreshAccessToken(refreshToken string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	return postToken(form)
}

// postToken performs a POST to the OAuth token endpoint.
func postToken(form url.Values) (*Token, error) {
	req, err := http.NewRequest("POST", oauthBaseURL+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("schwab: creating token request: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuth())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := netty.FastHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("schwab: token request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("schwab: reading token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("schwab: token request returned %d: %s", resp.StatusCode, body)
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("schwab: parsing token response: %w", err)
	}
	now := clocky.Now()
	t := &Token{
		AccessToken:      tr.AccessToken,
		RefreshToken:     tr.RefreshToken,
		ExpiresAt:        now + clocky.Time(clocky.Duration(tr.ExpiresIn)*clocky.Second),
		RefreshExpiresAt: now + clocky.Time(7*24*clocky.Hour),
	}
	// preserve account info from previous token
	if old := getToken(); old != nil {
		t.AccountHash = old.AccountHash
		t.AccountNumber = old.AccountNumber
	}
	setToken(t)
	return t, nil
}

// getAccessToken returns a valid access token, refreshing if necessary.
func getAccessToken() (string, error) {
	t := getToken()
	if t == nil {
		return "", fmt.Errorf("schwab: no token found; authorize at: %s", AuthorizeURL())
	}
	now := clocky.Now()
	// refresh 60 seconds before expiry to avoid races
	if now < t.ExpiresAt-clocky.Time(60*clocky.Second) {
		return t.AccessToken, nil
	}
	if now >= t.RefreshExpiresAt {
		return "", fmt.Errorf("schwab: refresh token expired; re-authorize at: %s", AuthorizeURL())
	}
	refreshed, err := refreshAccessToken(t.RefreshToken)
	if err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}
