package coinbase

import (
	"dropbear/ds"
	"io"
	"net/http"
	"time"
)

// HTTPWarmupDaemon keeps n FastHTTPClient connections to Coinbase warm.
// The teddy framework calls this once in live mode, when Coinbase is being used.
func HTTPWarmupDaemon(n int) {
	ticker := time.NewTicker(20 * time.Second)
	for range ticker.C {
		for range n {
			go pingFastAPI()
		}
	}
}

func pingFastAPI() {
	req, err := http.NewRequest("GET", "https://api.coinbase.com/v2/time", nil)
	if err != nil {
		return
	}
	resp, err := ds.FastHTTPClient.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
