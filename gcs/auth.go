package gcs

import (
	"dropbear/clocky"
	"dropbear/netty"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

var gAuth auth

type auth struct {
	mu     sync.Mutex
	tok    string
	expiry clocky.Time
}

// token returns memoized auth token, refreshing if necessary.
func (a *auth) token() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tok != "" && clocky.Now().Before(a.expiry.Add(-30*clocky.Second)) {
		return a.tok, nil
	}
	token, expiry, err := fetchTokenADC()
	if err != nil {
		var err2 error
		token, expiry, err2 = fetchTokenMetadata()
		if err2 != nil {
			return "", fmt.Errorf("gcs: could not get auth token; try running `gcloud auth application-default login`\n\tADC error: %v\n\tMetadata server error: %v", err, err2)
		}
	}
	a.tok = token
	a.expiry = expiry
	return token, nil
}

// fetchTokenADC gets auth token made by `gcloud auth application-default login`.
// this is what you'll use on your local machine during development.
func fetchTokenADC() (string, clocky.Time, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", 0, err
	}
	path := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	var creds struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", 0, err
	}
	if creds.RefreshToken == "" {
		return "", 0, errors.New("gcs: ADC missing refresh_token")
	}
	resp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"client_id":     {creds.ClientID},
		"client_secret": {creds.ClientSecret},
		"refresh_token": {creds.RefreshToken},
		"grant_type":    {"refresh_token"},
	})
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("oauth2 token: %s: %s", resp.Status, body)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, err
	}
	expiresIn := clocky.Duration(result.ExpiresIn) * clocky.Second
	expiresAt := clocky.Now().Add(expiresIn)
	return result.AccessToken, expiresAt, nil
}

// fetchTokenMetadata gets a token from the GCE metadata server.
// this only works when running on google compute engine in prod.
func fetchTokenMetadata() (string, clocky.Time, error) {
	req, _ := http.NewRequest("GET", "http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/token", nil)
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := netty.GCSHTTPClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("metadata server: %s", resp.Status)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, err
	}
	expiresIn := clocky.Duration(result.ExpiresIn) * clocky.Second
	expiresAt := clocky.Now().Add(expiresIn)
	return result.AccessToken, expiresAt, nil
}
