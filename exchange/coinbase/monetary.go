package coinbase

type MonetaryAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}
