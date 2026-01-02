package teddy

import (
	"dropbear/broker/coinbase"
	"dropbear/broker/kraken"
	"dropbear/decimal"
	"dropbear/ds"
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

type Broker struct {
	Lock     sync.RWMutex
	Broker   ds.Broker
	Holdings *Holdings
	Orders   *Orders
	Pairs    *Pairs
	MakerFee decimal.Decimal // percent of notional commission for limit orders that don't take liquidity
	TakerFee decimal.Decimal // percent of notional commission for marketable orders that take liquidity
	Rebate   decimal.Decimal // percent usdc rebate on fees paid (e.g. 0.25 for 25% rebate)
	Fees     decimal.Decimal // total fees paid on this broker since starting teddy
	OnReady  func()
}

func newBroker(broker ds.Broker) *Broker {
	if gRunning {
		loggy.Fatalf("cannot create new broker %ss while teddy is running", broker)
	}
	ex := &Broker{
		Broker:  broker,
		OnReady: func() {},
	}
	switch broker {
	case ds.BrokerCoinbase:
		if Live {
			err := ex.fetchCoinbaseFeeRates()
			if err != nil {
				loggy.Fatalf("could not determine fee rates for %v: %v", broker, err)
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

func (ex *Broker) String() string {
	return ex.Broker.String()
}

func (ex *Broker) run() {
	switch ex.Broker {
	case ds.BrokerCoinbase:
		go coinbase.HTTPWarmupDaemon(3) // keep 3 connections warm
		go ex.Orders.coinbaseOrderUpdateDaemon()
		if !Paper {
			go ex.Orders.coinbaseOrderSyncDaemon()
		}
		go ex.coinbaseFeeRatesDaemon()
	case ds.BrokerKraken:
		go kraken.HTTPWarmupDaemon(3) // keep 3 connections warm
	}
}

func (ex *Broker) coinbaseFeeRatesDaemon() {
	for {
		Hibernate()
		err := ex.fetchCoinbaseFeeRates()
		if err != nil {
			log.Printf("error refreshing fee rates for %v: %v", ex.Broker, err)
		}
	}
}

func (ex *Broker) fetchCoinbaseFeeRates() error {
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

func compareBrokers(a, b *Broker) int {
	if a.Broker < b.Broker {
		return -1
	}
	if a.Broker > b.Broker {
		return +1
	}
	return 0
}
