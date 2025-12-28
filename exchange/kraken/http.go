package kraken

import (
	"dropbear/ds"
	"io"
	"net/http"
	"time"
)

// HTTPWarmupDaemon keeps n FastHTTPClient connections to Kraken warm.
// The teddy framework calls this once in live mode, when Kraken is being used.
func HTTPWarmupDaemon(n int) {
	ticker := time.NewTicker(20 * time.Second)
	for range ticker.C {
		for range n {
			go pingFastAPI()
		}
	}
}

func pingFastAPI() {
	req, err := http.NewRequest("GET", "https://api.kraken.com/0/public/Time", nil)
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
