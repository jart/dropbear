package coinbase

import (
	"crypto/ed25519"
	"dropbear/loggy"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	flagKey = flag.String("coinbase-key", "coinbase.json", "path to coinbase ed25519 cdp key json file")
)

var (
	keyOnce    sync.Once
	keySaveID  string
	keySaveKey ed25519.PrivateKey
)

func getKey() (string, ed25519.PrivateKey) {
	keyOnce.Do(func() {
		data, err := os.ReadFile(*flagKey)
		if err != nil {
			loggy.Fatalf("reading coinbase key file: %w", err)
		}
		var config struct {
			ID         string `json:"id"`
			PrivateKey string `json:"privateKey"`
		}
		if err := json.Unmarshal(data, &config); err != nil {
			loggy.Fatalf("parsing coinbase key file: %w", err)
		}
		keyBytes, err := base64.StdEncoding.DecodeString(config.PrivateKey)
		if err != nil {
			loggy.Fatalf("decoding coinbase private key base64: %w", err)
		}
		if len(keyBytes) != ed25519.PrivateKeySize {
			loggy.Fatalf("private coinbase key wrong size: got %d, want %d", len(keyBytes), ed25519.PrivateKeySize)
		}
		privateKey := ed25519.PrivateKey(keyBytes)
		keySaveID = config.ID
		keySaveKey = privateKey
	})
	return keySaveID, keySaveKey
}

// generateJWT creates a string for authenticating a request.
func generateJWT(uri string, expirationSeconds int64) string {
	kid, key := getKey()
	now := time.Now().Unix()
	nonce := strconv.FormatUint(generateNonce(), 10)
	header := map[string]string{
		"alg":   "EdDSA",
		"kid":   kid,
		"nonce": nonce,
		"typ":   "JWT",
	}
	headerJSON, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := map[string]any{
		"aud": []string{""},
		"exp": now + expirationSeconds,
		"iss": "coinbase-cloud",
		"nbf": now,
		"sub": kid,
	}
	if uri != "" {
		payload["uris"] = []string{uri}
	}
	payloadJSON, _ := json.Marshal(payload)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	message := headerB64 + "." + payloadB64
	signature := ed25519.Sign(key, []byte(message))
	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)
	return message + "." + signatureB64
}
