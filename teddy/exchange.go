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
	OnReady  func()
}

func newExchange(exchange ds.Exchange) *Exchange {
	if gRunning {
		loggy.Fatalf("cannot create new exchange %ss while teddy is running", exchange)
	}
	ex := &Exchange{
		Exchange: exchange,
		OnReady:  func() {},
	}
	switch exchange {
	case ds.ExchangeCoinbase:
		if Live {
			err := ex.fetchCoinbaseFeeRates()
			if err != nil {
				loggy.Fatalf("could not determine fee rates for %v: %v", exchange, err)
			}
		} else {
			ex.MakerFee = *flagCoinbaseMakerFee
			ex.TakerFee = *flagCoinbaseTakerFee
			ex.Rebate = *flagCoinbaseRebate
		}
	}
	ex.Holdings = newHoldings(ex)
	ex.Orders = newOrders(ex)
	ex.Pairs = newPairs(ex)
	return ex
}

func (ex *Exchange) String() string {
	return ex.Exchange.String()
}

func (ex *Exchange) run() {
	switch ex.Exchange {
	case ds.ExchangeCoinbase:
		go coinbase.HTTPWarmupDaemon(3) // keep 3 connections warm
		go ex.Orders.coinbaseOrderUpdateDaemon()
		go ex.coinbaseFeeRatesDaemon()
	}
}

func (ex *Exchange) coinbaseFeeRatesDaemon() {
	for {
		Hibernate()
		err := ex.fetchCoinbaseFeeRates()
		if err != nil {
			log.Printf("error refreshing fee rates for %v: %v", ex.Exchange, err)
		}
	}
}

func (ex *Exchange) fetchCoinbaseFeeRates() error {
	summary, err := CoinbaseClient.GetTransactionSummary()
	if err != nil {
		return err
	}
	ex.MakerFee.Store(decimal.Parse(summary.FeeTier.MakerFeeRate))
	ex.TakerFee.Store(decimal.Parse(summary.FeeTier.TakerFeeRate))
	if strings.Contains(summary.FeeTier.PricingTier, "VIP") {
		ex.Rebate.Store(decimal.Parse("0.25"))
	} else {
		ex.Rebate.Store(decimal.Zero)
	}
	return nil
}

func compareExchanges(a, b *Exchange) int {
	if a.Exchange < b.Exchange {
		return -1
	}
	if a.Exchange > b.Exchange {
		return +1
	}
	return 0
}
