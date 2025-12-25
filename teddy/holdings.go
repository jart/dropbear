package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/coinbase"
	"dropbear/loggy"
	"sync"
)

type Holdings struct {
	lock          sync.RWMutex
	exchange      *Exchange
	holdingsMap   map[string]*Holding
	holdingsArray []*Holding
}

func newHoldings(exchange *Exchange) *Holdings {
	hs := &Holdings{
		exchange:      exchange,
		holdingsMap:   make(map[string]*Holding),
		holdingsArray: make([]*Holding, 0),
	}
	if Live {
		switch exchange.Exchange {
		case ds.ExchangeCoinbase:
			hs.fetchCoinbaseHoldings()
		}
	}
	return hs
}

func (hs *Holdings) Lookup(symbol string) *Holding {
	hs.lock.RLock()
	ho := hs.holdingsMap[symbol]
	hs.lock.RUnlock()
	return ho
}

func (hs *Holdings) Get(symbol string) *Holding {
	ho := hs.Lookup(symbol)
	if ho == nil {
		hs.lock.Lock()
		ho = hs.holdingsMap[symbol]
		if ho == nil {
			ho = newHolding(hs.exchange, symbol)
			hs.holdingsMap[symbol] = ho
			hs.holdingsArray = append(hs.holdingsArray, ho)
		}
		hs.lock.Unlock()
	}
	return ho
}

// All returns all Holding objects in insertion order.
func (hs *Holdings) All() []*Holding {
	hs.lock.RLock()
	defer hs.lock.RUnlock()
	result := make([]*Holding, 0, len(hs.holdingsArray))
	for _, holding := range hs.holdingsArray {
		result = append(result, holding)
	}
	return result
}

// GetEquityUSD returns the total equity of all holdings in USD on this exchange.
func (hs *Holdings) GetEquityUSD() decimal.Decimal {
	total := decimal.Zero
	for _, holding := range hs.All() {
		price := holding.Exchange.GetPriceUSD(holding.Symbol)
		holding.Lock.RLock()
		value := holding.Quantity.Mul(price)
		holding.Lock.RUnlock()
		total = total.Add(value)
	}
	return total
}

// GetInvestedUSD returns the total invested (non-cash) equity of all holdings in USD on this exchange.
func (hs *Holdings) GetInvestedUSD() decimal.Decimal {
	total := decimal.Zero
	for _, holding := range hs.All() {
		if holding.IsCash {
			continue
		}
		price := holding.Exchange.GetPriceUSD(holding.Symbol)
		holding.Lock.RLock()
		value := holding.Quantity.Mul(price)
		holding.Lock.RUnlock()
		total = total.Add(value)
	}
	return total
}

type initialHolding struct {
	Exchange ds.Exchange
	Symbol   string
	Quantity decimal.Decimal
}

func captureInitialHoldings() {
	haveSomething := false
	for _, exchange := range Exchanges.All() {
		for _, holding := range exchange.Holdings.All() {
			holding.Lock.RLock()
			qty := holding.Quantity
			holding.Lock.RUnlock()
			if qty.IsPositive() {
				haveSomething = true
				gInitialHoldings = append(gInitialHoldings, initialHolding{
					Exchange: exchange.Exchange,
					Symbol:   holding.Symbol,
					Quantity: qty,
				})
			}
		}
	}
	if !haveSomething {
		loggy.Fatalf("no initial cash or holdings set for backtest; forgot to call teddy.SetBalance()?")
	}
}

func (hs *Holdings) fetchCoinbaseHoldings() {
	portfolioUUID := coinbase.GetPortfolioUUID(CoinbaseClient)
	breakdown, err := CoinbaseClient.GetPortfolioBreakdown(portfolioUUID)
	if err != nil {
		loggy.Fatalf("coinbase: error fetching portfolio breakdown: %v", err)
	}
	for _, pos := range breakdown.Breakdown.SpotPositions {
		if pos.AccountType != "ACCOUNT_TYPE_FIAT" && pos.AccountType != "ACCOUNT_TYPE_WALLET" {
			continue
		}
		holding := hs.Lookup(pos.Asset)
		if holding == nil {
			continue
		}
		holding.Lock.Lock()
		holding.IsCash = pos.IsCash
		if pos.IsCash {
			holding.Quantity = decimal.Parse(pos.TotalBalanceFiat.String())
			holding.Available = decimal.Parse(pos.AvailableToTradeFiat.String())
		} else {
			holding.Quantity = decimal.Parse(pos.TotalBalanceCrypto.String())
			holding.Available = decimal.Parse(pos.AvailableToTradeCrypto.String())
			err := CoinbaseClient.SyncTransactions(pos.Asset)
			if err != nil {
				loggy.Fatalf("coinbase: error syncing transactions for asset %s: %v", pos.Asset, err)
			}
			holding.Lots, err = CoinbaseClient.GetLots(pos.Asset, GetCostBasisMethod())
			if err != nil {
				loggy.Fatalf("coinbase: error fetching lots for asset %s: %v", pos.Asset, err)
			}
		}
		holding.Check()
		holding.Lock.Unlock()
	}
}
