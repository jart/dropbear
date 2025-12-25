package coinbase

type EventUpdate struct {
	Side        string `json:"side"`
	PriceLevel  string `json:"price_level"`
	NewQuantity string `json:"new_quantity"`
}

type EventTrade struct {
	ProductID string `json:"product_id"`
	TradeID   string `json:"trade_id"`
	Price     string `json:"price"`
	Size      string `json:"size"`
	Time      string `json:"time"` // e.g. 2025-12-08T17:55:52.678124Z
	Side      string `json:"side"` // e.g. BUY, SELL (what the maker did)
}

type Event struct {
	Type    string        `json:"type"`
	Updates []EventUpdate `json:"updates"`
	Trades  []EventTrade  `json:"trades"`
}

type Events struct {
	Channel string  `json:"channel"`
	Type    string  `json:"type"` // for subscriptions response
	Events  []Event `json:"events"`
}
