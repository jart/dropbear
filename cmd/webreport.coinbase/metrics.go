package main

import (
	"database/sql"
	"dropbear/broker/coinbase"
	"dropbear/clocky"
	"dropbear/db"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/teddy/metrics"
	"fmt"
	"log"
	"math"
	"sort"
)

// Transaction represents a transaction from the database.
type Transaction struct {
	ID           string
	Type         string
	Status       string
	CreatedAt    clocky.Time
	Amount       decimal.Decimal // crypto amount (positive = credit, negative = debit)
	NativeAmount decimal.Decimal // USD value at time of transaction
	Currency     string
	FillSide     string // "buy" or "sell" for advanced_trade_fill
	FillPrice    decimal.Decimal
	Commission   decimal.Decimal // fee paid for this transaction
}

// PortfolioSnapshot represents portfolio state at a point in time.
type PortfolioSnapshot struct {
	Time           clocky.Time
	StrategyValue  decimal.Decimal
	BenchmarkValue decimal.Decimal
}

// ReportMetrics contains calculated performance metrics.
type ReportMetrics struct {
	TotalReturn     float64 // percentage
	CAGR            float64 // percentage
	Sharpe          float64
	MaxDrawdown     float64 // percentage
	BenchmarkReturn float64 // percentage
	BenchmarkCAGR   float64 // percentage
	BenchmarkSharpe float64
	BenchmarkMaxDD  float64 // percentage
	CurrentValue    decimal.Decimal
	StartValue      decimal.Decimal
	FeesPaid        decimal.Decimal
	TotalRealized   decimal.Decimal // realized P/L from sells
	TotalUnrealized decimal.Decimal // unrealized P/L on current holdings
	AssetBalance    decimal.Decimal // current crypto holdings
	USDBalance      decimal.Decimal // current USD balance
	AssetPrice      decimal.Decimal // current asset price
}

// PriceCache caches historical prices for efficient lookup.
type PriceCache struct {
	candles []*indicators.Candle // sorted by time ascending
}

// NewPriceCache creates a price cache from candles.
func NewPriceCache(candles []*indicators.Candle) *PriceCache {
	// ensure sorted by time
	sorted := make([]*indicators.Candle, len(candles))
	copy(sorted, candles)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Start < sorted[j].Start
	})
	return &PriceCache{candles: sorted}
}

// GetPrice returns the price at or before the given time.
// Returns the close price of the most recent candle before or at the time.
func (p *PriceCache) GetPrice(t clocky.Time) decimal.Decimal {
	if len(p.candles) == 0 {
		return decimal.Zero
	}
	// binary search for the candle at or before time t
	idx := sort.Search(len(p.candles), func(i int) bool {
		return p.candles[i].Start > t
	})
	if idx == 0 {
		// t is before all candles, use first candle
		return p.candles[0].Close
	}
	return p.candles[idx-1].Close
}

// fetchTransactions queries transactions from the database.
func fetchTransactions(asset string, genesis clocky.Time) ([]Transaction, error) {
	database := db.Get()
	rows, err := database.Query(`
		SELECT id, type, status, created_at, amount, native_amount, currency,
		       COALESCE(fill_side, ''), COALESCE(fill_price, ''), COALESCE(fill_commission, '')
		FROM coinbase_transactions
		WHERE currency = :currency
		  AND created_at >= :genesis
		  AND status = 'completed'
		ORDER BY created_at ASC
	`, sql.Named("currency", asset), sql.Named("genesis", int64(genesis)))
	if err != nil {
		return nil, fmt.Errorf("querying transactions: %w", err)
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var t Transaction
		var amountStr, nativeStr, fillPriceStr, commissionStr string
		var createdAt int64
		err := rows.Scan(&t.ID, &t.Type, &t.Status, &createdAt,
			&amountStr, &nativeStr, &t.Currency, &t.FillSide, &fillPriceStr, &commissionStr)
		if err != nil {
			return nil, fmt.Errorf("scanning transaction: %w", err)
		}
		t.CreatedAt = clocky.Time(createdAt)
		t.Amount = decimal.Parse(amountStr)
		t.NativeAmount = decimal.Parse(nativeStr)
		if fillPriceStr != "" {
			t.FillPrice = decimal.Parse(fillPriceStr)
		}
		if commissionStr != "" {
			t.Commission = decimal.Parse(commissionStr)
		}
		transactions = append(transactions, t)
	}
	return transactions, rows.Err()
}

// fetchAllCandles fetches all candles from genesis to now with pagination.
// Note: Coinbase API uses confusing naming where "start" is the most recent
// timestamp and "end" is the least recent timestamp.
func fetchAllCandles(client *coinbase.Client, productID string, genesis clocky.Time, quantum clocky.Duration) ([]*indicators.Candle, error) {
	// determine granularity based on quantum
	granularity := coinbase.CandleGranularityHour
	if quantum >= clocky.Day {
		granularity = coinbase.CandleGranularityDay
	} else if quantum >= 6*clocky.Hour {
		granularity = coinbase.CandleGranularitySixHours
	} else if quantum >= 2*clocky.Hour {
		granularity = coinbase.CandleGranularityTwoHours
	} else if quantum >= clocky.Hour {
		granularity = coinbase.CandleGranularityHour
	} else if quantum >= 30*clocky.Minute {
		granularity = coinbase.CandleGranularityThirtyMinutes
	} else if quantum >= 15*clocky.Minute {
		granularity = coinbase.CandleGranularityFifteenMinutes
	} else if quantum >= 5*clocky.Minute {
		granularity = coinbase.CandleGranularityFiveMinutes
	} else {
		granularity = coinbase.CandleGranularityMinute
	}

	var allCandles []*indicators.Candle
	now := clocky.Now()
	batchDuration := clocky.Duration(granularity) * 300 // slightly less than 350 for safety

	// Work backwards from now to genesis, then reverse the result
	// API: start=most recent, end=least recent
	mostRecent := now
	for mostRecent > genesis {
		leastRecent := max(mostRecent.Add(-batchDuration), genesis)

		// API: start=older timestamp, end=newer timestamp (despite confusing comments in candles.go)
		candles, err := client.GetCandles(productID, granularity, leastRecent, mostRecent, 350)
		if err != nil {
			return nil, fmt.Errorf("fetching candles: %w", err)
		}
		if len(candles) == 0 {
			break
		}

		// prepend candles (since we're working backwards)
		allCandles = append(candles, allCandles...)
		log.Printf("fetched %d candles for %s (total: %d)", len(candles), productID, len(allCandles))

		// move backwards - oldest candle we got minus one granularity
		mostRecent = candles[0].Start.Add(-clocky.Duration(granularity))
	}

	return allCandles, nil
}

// calculateMetrics computes portfolio value over time and performance metrics.
func calculateMetrics(
	client *coinbase.Client,
	asset string,
	genesis clocky.Time,
	quantum clocky.Duration,
) ([]PortfolioSnapshot, *ReportMetrics, error) {
	// sync transactions first
	if err := client.SyncTransactions(asset); err != nil {
		return nil, nil, fmt.Errorf("syncing transactions: %w", err)
	}

	// also sync USD transactions (for fiat deposits/withdrawals)
	if err := client.SyncTransactions("USD"); err != nil {
		log.Printf("warning: failed to sync USD transactions: %v", err)
	}

	// fetch transactions from database
	transactions, err := fetchTransactions(asset, genesis)
	if err != nil {
		return nil, nil, err
	}
	log.Printf("loaded %d %s transactions since %s", len(transactions), asset, genesis)

	// fetch historical prices
	productID := asset + "-USD"
	candles, err := fetchAllCandles(client, productID, genesis, quantum)
	if err != nil {
		return nil, nil, err
	}
	if len(candles) == 0 {
		return nil, nil, fmt.Errorf("no price data available for %s", productID)
	}
	prices := NewPriceCache(candles)
	log.Printf("loaded %d candles for %s", len(candles), productID)

	// get current balances for initial state
	accounts, err := client.GetAccounts()
	if err != nil {
		return nil, nil, fmt.Errorf("getting accounts: %w", err)
	}

	var currentAssetBalance, currentUSDBalance decimal.Decimal
	for _, acc := range accounts {
		if acc.Currency == asset {
			currentAssetBalance = decimal.Parse(acc.AvailableBalance.Value)
		} else if acc.Currency == "USD" {
			currentUSDBalance = decimal.Parse(acc.AvailableBalance.Value)
		}
	}

	// replay transactions to get initial state at genesis
	// work backwards: start with current balance and subtract transactions
	assetBalance := currentAssetBalance
	usdBalance := currentUSDBalance

	// reverse replay to get genesis state
	// for trades, we need to also reverse the USD side
	for i := len(transactions) - 1; i >= 0; i-- {
		t := transactions[i]
		// undo the crypto effect
		assetBalance = assetBalance.Sub(t.Amount)

		// undo the USD effect for trades
		if t.Type == "advanced_trade_fill" || t.Type == "buy" || t.Type == "sell" || t.Type == "trade" {
			// when we bought crypto, we spent USD (native_amount is negative for our USD)
			// when we sold crypto, we received USD (native_amount is positive for our USD)
			// Amount is + for buys (we receive crypto), - for sells (we send crypto)
			// So USD change is opposite: - for buys, + for sells = -native_amount approximately
			// native_amount is the fiat equivalent, which should be close to what we paid/received
			usdBalance = usdBalance.Add(t.NativeAmount) // undo by adding back what we spent/received
		}
	}

	// now assetBalance and usdBalance are the balances at genesis
	genesisAssetBalance := assetBalance
	genesisUSDBalance := usdBalance
	genesisPrice := prices.GetPrice(genesis)
	genesisValue := genesisAssetBalance.Mul(genesisPrice).Add(genesisUSDBalance)

	if !genesisValue.IsPositive() {
		return nil, nil, fmt.Errorf("genesis portfolio value is not positive: %s", genesisValue)
	}

	log.Printf("genesis state: %s %s + %s USD @ %s = %s USD total",
		genesisAssetBalance, asset, genesisUSDBalance, genesisPrice, genesisValue)

	// initialize benchmark: buy and hold from genesis
	// benchmark starts with same USD value, all in the asset
	benchmarkQty := genesisValue.Div(genesisPrice)

	// create equity trackers
	strategyEquity := metrics.NewEquity(quantum)
	benchmarkEquity := metrics.NewEquity(quantum)

	// sample at each quantum from genesis to now
	var snapshots []PortfolioSnapshot
	now := clocky.Now()

	// reset to genesis state for forward replay
	assetBalance = genesisAssetBalance
	usdBalance = genesisUSDBalance
	txIdx := 0
	totalFees := decimal.Zero
	totalRealized := decimal.Zero
	lots := ds.NewLots(ds.CostBasisMethodLIFO)

	// seed lots with genesis holdings at genesis price
	if genesisAssetBalance.IsPositive() {
		lots.Add(genesis, genesisAssetBalance, genesisAssetBalance.Mul(genesisPrice))
	}

	for t := genesis; t <= now; t = t.Add(quantum) {
		// apply transactions up to this time
		for txIdx < len(transactions) && transactions[txIdx].CreatedAt <= t {
			tx := transactions[txIdx]

			// accumulate fees
			totalFees = totalFees.Add(tx.Commission)

			// detect capital events (external deposits/withdrawals)
			// and adjust benchmark accordingly
			switch tx.Type {
			case "send":
				if tx.Amount.IsPositive() {
					// receiving crypto = deposit
					// add same value to benchmark
					depositValue := tx.Amount.Mul(prices.GetPrice(tx.CreatedAt))
					benchmarkQty = benchmarkQty.Add(tx.Amount)
					log.Printf("deposit detected: %s %s (~%s USD), benchmark adjusted",
						tx.Amount, asset, depositValue)
					// add to lots at current price
					lots.Add(tx.CreatedAt, tx.Amount, depositValue)
				}
				// apply to strategy
				assetBalance = assetBalance.Add(tx.Amount)

			case "fiat_deposit", "fiat_withdrawal":
				// direct fiat moves - adjust benchmark by same USD amount
				usdAmount := tx.NativeAmount.Abs()
				price := prices.GetPrice(tx.CreatedAt)
				if price.IsPositive() {
					if tx.Type == "fiat_deposit" {
						benchmarkQty = benchmarkQty.Add(usdAmount.Div(price))
						usdBalance = usdBalance.Add(usdAmount)
					} else {
						benchmarkQty = benchmarkQty.Sub(usdAmount.Div(price))
						usdBalance = usdBalance.Sub(usdAmount)
					}
				}

			case "advanced_trade_fill", "buy", "sell", "trade":
				// trading activity - apply crypto and USD changes to strategy, not benchmark
				assetBalance = assetBalance.Add(tx.Amount)
				// USD change is opposite of crypto: buy crypto = spend USD, sell crypto = receive USD
				// NativeAmount tracks the fiat equivalent
				usdBalance = usdBalance.Sub(tx.NativeAmount)

				// track lots for realized P/L
				if tx.Amount.IsPositive() {
					// buy: add lot
					lots.Add(tx.CreatedAt, tx.Amount, tx.NativeAmount)
				} else {
					// sell: consume lots and calculate realized P/L
					sellQty := tx.Amount.Neg()
					price := prices.GetPrice(tx.CreatedAt)
					proceeds := sellQty.Mul(price)
					costBasis := lots.Consume(sellQty, price)
					totalRealized = totalRealized.Add(proceeds.Sub(costBasis))
				}

			default:
				// other types (interest, rewards, etc.) - treat as deposits
				if tx.Amount.IsPositive() {
					assetBalance = assetBalance.Add(tx.Amount)
					benchmarkQty = benchmarkQty.Add(tx.Amount)
					// add to lots at zero cost (free)
					lots.Add(tx.CreatedAt, tx.Amount, decimal.Zero)
				}
			}
			txIdx++
		}

		// calculate current values (crypto + USD)
		price := prices.GetPrice(t)
		strategyValue := assetBalance.Mul(price).Add(usdBalance)
		benchmarkValue := benchmarkQty.Mul(price)

		// sample if it's time
		if strategyEquity.ShouldSample(t) && strategyValue.IsPositive() && benchmarkValue.IsPositive() {
			strategyEquity.Sample(t, strategyValue)
			benchmarkEquity.Sample(t, benchmarkValue)

			snapshots = append(snapshots, PortfolioSnapshot{
				Time:           t,
				StrategyValue:  strategyValue,
				BenchmarkValue: benchmarkValue,
			})
		}
	}

	if len(snapshots) < 2 {
		return nil, nil, fmt.Errorf("not enough data points for metrics calculation")
	}

	// calculate final values
	finalSnapshot := snapshots[len(snapshots)-1]
	startValue := snapshots[0].StrategyValue
	endValue := finalSnapshot.StrategyValue
	benchmarkStart := snapshots[0].BenchmarkValue
	benchmarkEnd := finalSnapshot.BenchmarkValue
	currentPrice := prices.GetPrice(now)

	// calculate unrealized P/L: current value - cost basis of remaining lots
	currentMarketValue := assetBalance.Mul(currentPrice)
	remainingCostBasis := lots.GetCostBasis(assetBalance, currentPrice)
	totalUnrealized := currentMarketValue.Sub(remainingCostBasis)

	// calculate returns
	totalReturn := endValue.Sub(startValue).Div(startValue).Float64() * 100
	benchmarkReturn := benchmarkEnd.Sub(benchmarkStart).Div(benchmarkStart).Float64() * 100

	// calculate CAGR
	duration := finalSnapshot.Time.Sub(genesis)
	yearsElapsed := float64(duration) / float64(360*clocky.Day)
	if yearsElapsed > 0 {
		strategyCAGR := (math.Pow(endValue.Float64()/startValue.Float64(), 1/yearsElapsed) - 1) * 100
		benchmarkCAGR := (math.Pow(benchmarkEnd.Float64()/benchmarkStart.Float64(), 1/yearsElapsed) - 1) * 100

		result := &ReportMetrics{
			TotalReturn:     totalReturn,
			CAGR:            strategyCAGR,
			Sharpe:          strategyEquity.Sharpe(0.0487), // 4.87% risk-free rate
			MaxDrawdown:     strategyEquity.MaxDrawdown() * 100,
			BenchmarkReturn: benchmarkReturn,
			BenchmarkCAGR:   benchmarkCAGR,
			BenchmarkSharpe: benchmarkEquity.Sharpe(0.0487),
			BenchmarkMaxDD:  benchmarkEquity.MaxDrawdown() * 100,
			CurrentValue:    endValue,
			StartValue:      startValue,
			FeesPaid:        totalFees,
			TotalRealized:   totalRealized,
			TotalUnrealized: totalUnrealized,
			AssetBalance:    assetBalance,
			USDBalance:      usdBalance,
			AssetPrice:      currentPrice,
		}
		return snapshots, result, nil
	}

	return snapshots, &ReportMetrics{
		TotalReturn:     totalReturn,
		BenchmarkReturn: benchmarkReturn,
		CurrentValue:    endValue,
		StartValue:      startValue,
		FeesPaid:        totalFees,
		TotalRealized:   totalRealized,
		TotalUnrealized: totalUnrealized,
		AssetBalance:    assetBalance,
		USDBalance:      usdBalance,
		AssetPrice:      currentPrice,
	}, nil
}
