package coinbase

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	flagKey = flag.String("coinbase-key", os.ExpandEnv("$HOME/.coinbase.key"), "path to coinbase ed25519 cdp key json file")
)

type Key struct {
	ID  string
	Key ed25519.PrivateKey
}

// LoadKey loads a coinbase ed25519 key from a json file.
// File should contain {"id": "...", "privateKey": "..."} where:
// - id is the key UUID string
// - privateKey is the base64-encoded ed25519 private key
func LoadKey(path string) (*Key, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading coinbase key file %s: %w", path, err)
	}
	var config struct {
		ID         string `json:"id"`
		PrivateKey string `json:"privateKey"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing coinbase key file %s: %w", path, err)
	}
	keyBytes, err := base64.StdEncoding.DecodeString(config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("decoding coinbase key file %s: %w", path, err)
	}
	if len(keyBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid coinbase key size in file %s: %d", path, len(keyBytes))
	}
	return &Key{
		ID:  config.ID,
		Key: ed25519.PrivateKey(keyBytes),
	}, nil
}

// MustLoadKey loads a coinbase ed25519 key from a json file or panics.
func MustLoadKey(path string) *Key {
	key, err := LoadKey(path)
	if err != nil {
		panic(err)
	}
	return key
}

// generateJWT creates a string for authenticating a request.
func (key *Key) GenerateJWT(uri string, expirationSeconds int64) string {
	now := time.Now().Unix()
	headerJSON, _ := json.Marshal(map[string]string{
		"alg":   "EdDSA",
		"kid":   key.ID,
		"nonce": strconv.FormatUint(generateNonce(), 10),
		"typ":   "JWT",
	})
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := map[string]any{
		"aud": []string{""},
		"exp": now + expirationSeconds,
		"iss": "coinbase-cloud",
		"nbf": now,
		"sub": key.ID,
	}
	if uri != "" {
		payload["uris"] = []string{uri}
	}
	payloadJSON, _ := json.Marshal(payload)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	message := headerB64 + "." + payloadB64
	signature := ed25519.Sign(key.Key, []byte(message))
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)
	return message + "." + signatureB64
}

var (
	keyOnce sync.Once
	keySave *Key
)

// GetDefaultKey loads and returns the default coinbase key.
// This may be configured using the `-coinbase-key FILE` flag.
func GetDefaultKey() *Key {
	keyOnce.Do(func() { keySave = MustLoadKey(*flagKey) })
	return keySave
}
