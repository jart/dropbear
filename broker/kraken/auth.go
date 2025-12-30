package kraken

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"dropbear/loggy"
	"encoding/base64"
	"encoding/json"
	"flag"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	flagKey = flag.String("kraken-key", os.ExpandEnv("$HOME/.kraken.key"), "path to kraken api key json file")
)

var (
	keyOnce       sync.Once
	keySaveID     string
	keySaveSecret []byte
)

var nonce atomic.Int64

func init() {
	nonce.Store(time.Now().UnixMilli())
}

func getKey() (string, []byte) {
	keyOnce.Do(func() {
		data, err := os.ReadFile(*flagKey)
		if err != nil {
			loggy.Fatalf("reading kraken key file: %v", err)
		}
		var config struct {
			ID     string `json:"id"`
			Secret string `json:"secret"`
		}
		if err := json.Unmarshal(data, &config); err != nil {
			loggy.Fatalf("parsing kraken key file: %v", err)
		}
		secretBytes, err := base64.StdEncoding.DecodeString(config.Secret)
		if err != nil {
			loggy.Fatalf("decoding kraken secret base64: %v", err)
		}
		keySaveID = config.ID
		keySaveSecret = secretBytes
	})
	return keySaveID, keySaveSecret
}

// generateNonce returns an always-increasing nonce for API requests.
func generateNonce() int64 {
	return nonce.Add(1)
}

// generateSignature creates the API-Sign header value.
// Algorithm: HMAC-SHA512 of (URI path + SHA256(nonce + POST data)) using base64-decoded secret.
func generateSignature(urlPath string, values url.Values, nonceValue int64) string {
	_, secret := getKey()

	// nonce + POST data
	postData := values.Encode()
	nonceStr := strconv.FormatInt(nonceValue, 10)
	sha := sha256.Sum256([]byte(nonceStr + postData))

	// URI path + SHA256 hash
	message := append([]byte(urlPath), sha[:]...)

	// HMAC-SHA512 with decoded secret
	mac := hmac.New(sha512.New, secret)
	mac.Write(message)
	signature := mac.Sum(nil)

	return base64.StdEncoding.EncodeToString(signature)
}

// getAPIKey returns the public API key for the API-Key header.
func getAPIKey() string {
	id, _ := getKey()
	return id
}
