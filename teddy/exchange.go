package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/coinbase"
	"dropbear/loggy"
	"log"
	"strings"
	"sync"
)

var (
	flagCoinbaseMakerFee = decimal.FlagBPS("coinbase-maker-fee", "2.5", "coinbase maker fee in basis points")
	flagCoinbaseTakerFee = decimal.FlagBPS("coinbase-taker-fee", "6.5", "coinbase taker fee in basis points")
	flagCoinbaseRebate   = decimal.FlagPercent("coinbase-rebate", "25", "coinbase percent rebate on fees")
)

type Exchange struct {
	Lock     sync.RWMutex
	Exchange ds.Exchange
	Holdings *Holdings
	Orders   *Orders
	Pairs    *Pairs
	MakerFee decimal.Decimal
	TakerFee decimal.Decimal
	Rebate   decimal.Decimal
	Fees     decimal.Decimal
}

func (ex *Exchange) String() string {
	return ex.Exchange.String()
}

// GetPriceUSD returns the price of the given symbol in USD on this exchange.
func (ex *Exchange) GetPriceUSD(symbol string) decimal.Decimal {
	switch symbol {
	case "USD", "USDT", "USDC":
		return decimal.One
	}
	ex.Pairs.lock.RLock()
	pair := ex.Pairs.pairsMap[symbol]
	if pair == nil {
		pair = ex.Pairs.pairsMap[symbol+"-USD"]
		if pair == nil {
			pair = ex.Pairs.pairsMap[symbol+"-USDT"]
			if pair == nil {
				pair = ex.Pairs.pairsMap[symbol+"-USDC"]
				if pair == nil {
					loggy.Fatalf("don't know to determine USD value of %s on %s", symbol, ex)
				}
			}
		}
	}
	ex.Pairs.lock.RUnlock()
	pair.Lock.RLock()
	price := pair.LastPrice
	pair.Lock.RUnlock()
	return price
}

func (ex *Exchange) refreshFeeRatesDaemon() {
	for {
		Hibernate()
		err := ex.refreshFeeRates()
		if err != nil {
			log.Printf("error refreshing fee rates for %v: %v", ex.Exchange, err)
		}
	}
}

func newExchange(exchange ds.Exchange) *Exchange {
	ex := &Exchange{Exchange: exchange}
	ex.Holdings = newHoldings(ex)
	ex.Orders = newOrders(ex)
	ex.Pairs = newPairs(ex)
	err := ex.refreshFeeRates()
	if err != nil {
		loggy.Fatalf("could not determine fee rates for %v: %v", exchange, err)
	}
	go ex.refreshFeeRatesDaemon()
	if exchange == ds.ExchangeCoinbase && Live {
		go coinbase.HTTPWarmupDaemon(3) // keep 3 connections warm
	}
	return ex
}

func (ex *Exchange) refreshFeeRates() error {
	if Live {
		switch ex.Exchange {
		case ds.ExchangeCoinbase:
			summary, err := CoinbaseClient.GetTransactionSummary()
			if err != nil {
				return err
			}
			ex.Lock.Lock()
			ex.MakerFee = decimal.Parse(summary.FeeTier.MakerFeeRate)
			ex.TakerFee = decimal.Parse(summary.FeeTier.TakerFeeRate)
			if strings.Contains(summary.FeeTier.PricingTier, "VIP") {
				ex.Rebate = decimal.Parse("0.25")
			} else {
				ex.Rebate = decimal.Zero
			}
			ex.Lock.Unlock()
		}
	} else {
		switch ex.Exchange {
		case ds.ExchangeCoinbase:
			ex.Lock.Lock()
			ex.MakerFee = *flagCoinbaseMakerFee
			ex.TakerFee = *flagCoinbaseTakerFee
			ex.Rebate = *flagCoinbaseRebate
			ex.Lock.Unlock()
		}
	}
	return nil
}
