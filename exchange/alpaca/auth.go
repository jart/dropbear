package alpaca

import (
	"dropbear/loggy"
	"encoding/json"
	"flag"
	"os"
	"sync"
)

var (
	flagKey = flag.String("alpaca-key", os.ExpandEnv("$HOME/.alpaca.key"), "path to alpaca key json file")
)

var (
	keyOnce       sync.Once
	keySaveKey    string
	keySaveSecret string
)

func GetKey() (string, string) {
	keyOnce.Do(func() {
		data, err := os.ReadFile(*flagKey)
		if err != nil {
			loggy.Fatalf("reading coinbase key file: %v", err)
		}
		var config struct {
			Key    string `json:"key"`
			Secret string `json:"secret"`
		}
		if err := json.Unmarshal(data, &config); err != nil {
			loggy.Fatalf("parsing coinbase key file: %v", err)
		}
		keySaveKey = config.Key
		keySaveSecret = config.Secret
	})
	return keySaveKey, keySaveSecret
}
