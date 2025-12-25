package coinbase

import (
	"dropbear/loggy"
	"flag"
	"sync"
)

var (
	portfolioFlag     = flag.String("coinbase-portfolio", "dropbear", "Name or UUID of Coinbase portfolio to use")
	thePortfolioUUID  string
	portfolioUUIDOnce sync.Once
)

// GetPortfolioUUID returns the UUID of the portfolio specified by -coinbase-portfolio flag.
// This is only needed when talking to APIs like portfolio breakdown, that require the UUID.
// Normally for trading APIs, the portfolio associated with your API key is what'll be used.
func GetPortfolioUUID(client *Client) string {
	portfolioUUIDOnce.Do(func() {
		flag := *portfolioFlag
		if looksLikeUUID(flag) {
			thePortfolioUUID = flag
			return
		}
		portfolios, err := client.GetPortfolios()
		if err != nil {
			loggy.Fatalf("coinbase: error fetching portfolios: %v", err)
		}
		for _, p := range portfolios.Portfolios {
			if p.Name == flag {
				thePortfolioUUID = p.UUID
				return
			}
		}
		loggy.Fatalf("coinbase: portfolio name not found: " + flag)
	})
	return thePortfolioUUID
}

func looksLikeUUID(s string) bool {
	return len(s) == 36
}
