package schwab

import (
	"dropbear/loggy"
	"encoding/json"
	"flag"
	"os"
	"sync"
)

var (
	flagKey = flag.String("schwab-key", os.ExpandEnv("$HOME/.schwab.key"), "path to schwab key json file")
)

type ApiKey struct {
	Callback string `json:"callback"`
	Key      string `json:"key"`
	Secret   string `json:"secret"`
}

var (
	keyOnce sync.Once
	keySave *ApiKey
)

func GetKey() *ApiKey {
	keyOnce.Do(func() {
		data, err := os.ReadFile(*flagKey)
		if err != nil {
			loggy.Fatalf("reading schwab key file: %v", err)
		}
		var key *ApiKey
		if err := json.Unmarshal(data, &key); err != nil {
			loggy.Fatalf("parsing schwab key file: %v", err)
		}
		keySave = key
	})
	return keySave
}
