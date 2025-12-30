package binanceusd

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"fmt"
)

// GetTrades retrieves recent trades for a specific symbol.
// limit defaults to 500 and has a maximum of 1000.
// results should be in chronological order.
// https://developers.binance.com/docs/binance-spot-api-docs/rest-api/market-data-endpoints#recent-trades-list
func (c *Client) GetTrades(symbol string, limit int) ([]*ds.Trade, error) {
	var raws []struct {
		ID           int64  `json:"id"`           // e.g. 7052444028
		Price        string `json:"price"`        // e.g. "88667.90"
		Qty          string `json:"qty"`          // e.g. "0.007"
		QuoteQty     string `json:"quoteQty"`     // e.g. "620.67"
		Time         int64  `json:"time"`         // e.g. 1766745324377
		IsBuyerMaker bool   `json:"isBuyerMaker"` // e.g. true
		IsRPITrade   bool   `json:"isRPITrade"`   // e.g. false
	}
	uri := "/fapi/v1/trades?symbol=" + symbol
	if limit > 0 {
		uri += fmt.Sprintf("&limit=%d", limit)
	}
	if err := c.GetJSON(uri, &raws); err != nil {
		return nil, err
	}
	trades := make([]*ds.Trade, 0, len(raws))
	for _, raw := range raws {
		side := ds.SideBuy
		if raw.IsBuyerMaker {
			side = ds.SideSell // we care about what taker did
		}
		trades = append(trades, &ds.Trade{
			Side:     side,
			Time:     clocky.Time(raw.Time * 1000),
			Price:    decimal.Parse(raw.Price),
			Quantity: decimal.Parse(raw.Qty),
		})
	}
	return trades, nil
}
