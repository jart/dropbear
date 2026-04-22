package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
)

// CryptoTransfer represents a crypto wallet funding transaction.
type CryptoTransfer struct {
	ID          string          `json:"id"`           // e.g. 904837e3-3b76-4c7d-8534-8e7a3e0a6ed1
	TxHash      string          `json:"tx_hash"`      // e.g. 0x123abc...
	Direction   string          `json:"direction"`    // e.g. INCOMING, OUTGOING
	Amount      decimal.Decimal `json:"amount"`       // e.g. 0.02200293
	USDValue    decimal.Decimal `json:"usd_value"`    // e.g. 1996.944374747
	Chain       string          `json:"chain"`        // e.g. BTC, SOL
	Asset       string          `json:"asset"`        // e.g. NATIVE
	FromAddress string          `json:"from_address"` // e.g. bc1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlz
	ToAddress   string          `json:"to_address"`   // e.g. ac1qxy2kgdygjrsqtzq2n0yrf2493p83kkfjhx0wlz
	Status      string          `json:"status"`       // e.g. PROCESSING, FAILED, COMPLETE
	CreatedAt   clocky.Time     `json:"created_at"`   // e.g. 2025-11-28T19:07:17.66569Z
	NetworkFee  decimal.Decimal `json:"network_fee"`  // e.g. 0.0001
	Fees        decimal.Decimal `json:"fees"`         // e.g. 0.0001
}

// GetCryptoTransfers retrieves crypto wallet funding transactions.
// This is useful for example to see if money is arriving from the Bitcoin network.
func (c *Client) GetCryptoTransfers() ([]CryptoTransfer, error) {
	var result []CryptoTransfer
	c.APITokenBucket.Get()
	err := c.RequestJSON(netty.BulkHttpClient, "GET", "/v2/wallets/transfers", false, nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
