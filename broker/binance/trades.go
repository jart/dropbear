package binance

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GetTrades retrieves recent trades for a specific symbol.
// limit defaults to 500 and has a maximum of 1000.
// results should be in chronological order.
// https://developers.binance.com/docs/binance-spot-api-docs/rest-api/market-data-endpoints#recent-trades-list
func (c *Client) GetTrades(symbol string, limit int) ([]*ds.Trade, error) {
	uri := "/api/v3/trades?symbol=" + symbol
	if limit > 0 {
		uri += fmt.Sprintf("&limit=%d", limit)
	}
	resp, err := c.Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch trades: %d %s", resp.StatusCode, string(body))
	}
	var raws []struct {
		ID           int64  `json:"id"`
		Price        string `json:"price"`    // e.g. "0.03339000"
		Qty          string `json:"qty"`      // e.g. "5.03390000"
		QuoteQty     string `json:"quoteQty"` // e.g. "0.16808192" (this is the total trade value in quote currency)
		Time         int64  `json:"time"`     // unix milliseconds
		IsBuyerMaker bool   `json:"isBuyerMaker"`
		// this means the trade was executed at the best available price, e.g. a market order.
		// otherwise it means it was a limit order that didn't match at the best price.
		// the reason why this is important is that fees may differ for maker vs taker orders.
		IsBestMatch bool `json:"isBestMatch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raws); err != nil {
		return nil, fmt.Errorf("failed to decode trades: %w", err)
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
