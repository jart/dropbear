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

// Holding represents a single asset holding.
type Holding struct {
	Currency string
	Balance  decimal.Decimal
	Price    decimal.Decimal // USD price (1.0 for USD)
	Value    decimal.Decimal // Balance * Price
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
	Holdings        []Holding       // all non-zero holdings
	BenchmarkAsset  string          // the asset used for benchmark (e.g., "ETH")
	BenchmarkPrice  decimal.Decimal // current price of benchmark asset
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
// If currency is empty, fetches all currencies.
func fetchTransactions(currency string, genesis clocky.Time) ([]Transaction, error) {
	database := db.Get()
	var rows *sql.Rows
	var err error
	if currency != "" {
		rows, err = database.Query(`
			SELECT id, type, status, created_at, amount, native_amount, currency,
			       COALESCE(fill_side, ''), COALESCE(fill_price, ''), COALESCE(fill_commission, '')
			FROM coinbase_transactions
			WHERE currency = :currency
			  AND created_at >= :genesis
			  AND status = 'completed'
			ORDER BY created_at ASC
		`, sql.Named("currency", currency), sql.Named("genesis", int64(genesis)))
	} else {
		rows, err = database.Query(`
			SELECT id, type, status, created_at, amount, native_amount, currency,
			       COALESCE(fill_side, ''), COALESCE(fill_price, ''), COALESCE(fill_commission, '')
			FROM coinbase_transactions
			WHERE created_at >= :genesis
			  AND status = 'completed'
			ORDER BY created_at ASC
		`, sql.Named("genesis", int64(genesis)))
	}
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
// benchmarkAsset is used for benchmark comparison (hold that asset).
// Portfolio value includes ALL assets.
func calculateMetrics(
	client *coinbase.Client,
	benchmarkAsset string,
	genesis clocky.Time,
	quantum clocky.Duration,
) ([]PortfolioSnapshot, *ReportMetrics, error) {
	// get all accounts and sync transactions for each
	accounts, err := client.GetAccounts()
	if err != nil {
		return nil, nil, fmt.Errorf("getting accounts: %w", err)
	}

	// sync transactions for all currencies with non-zero balance
	currencies := make(map[string]bool)
	currentBalances := make(map[string]decimal.Decimal)
	for _, acc := range accounts {
		bal := decimal.Parse(acc.AvailableBalance.Value)
		if !bal.IsZero() || acc.Currency == "USD" || acc.Currency == benchmarkAsset {
			currencies[acc.Currency] = true
			currentBalances[acc.Currency] = bal
			log.Printf("syncing %s transactions (balance: %s)", acc.Currency, bal)
			if err := client.SyncTransactions(acc.Currency); err != nil {
				log.Printf("warning: failed to sync %s transactions: %v", acc.Currency, err)
			}
		}
	}

	// fetch ALL transactions from database
	transactions, err := fetchTransactions("", genesis)
	if err != nil {
		return nil, nil, err
	}
	log.Printf("loaded %d transactions since %s", len(transactions), genesis)

	// collect unique currencies from transactions
	for _, tx := range transactions {
		currencies[tx.Currency] = true
	}

	// fetch historical prices for all non-USD currencies
	priceCaches := make(map[string]*PriceCache)
	for currency := range currencies {
		if currency == "USD" {
			continue
		}
		productID := currency + "-USD"
		candles, err := fetchAllCandles(client, productID, genesis, quantum)
		if err != nil {
			log.Printf("warning: failed to fetch candles for %s: %v", productID, err)
			continue
		}
		if len(candles) > 0 {
			priceCaches[currency] = NewPriceCache(candles)
			log.Printf("loaded %d candles for %s", len(candles), productID)
		}
	}

	// ensure we have price data for benchmark asset
	if _, ok := priceCaches[benchmarkAsset]; !ok {
		return nil, nil, fmt.Errorf("no price data available for benchmark asset %s-USD", benchmarkAsset)
	}

	// helper to get price for any currency at a time
	getPrice := func(currency string, t clocky.Time) decimal.Decimal {
		if currency == "USD" {
			return decimal.FromInt(1)
		}
		if cache, ok := priceCaches[currency]; ok {
			return cache.GetPrice(t)
		}
		return decimal.Zero
	}

	// helper to calculate total portfolio value
	calcPortfolioValue := func(balances map[string]decimal.Decimal, t clocky.Time) decimal.Decimal {
		total := decimal.Zero
		for currency, balance := range balances {
			price := getPrice(currency, t)
			total = total.Add(balance.Mul(price))
		}
		return total
	}

	// replay transactions backwards to get genesis state
	balances := make(map[string]decimal.Decimal)
	for currency, bal := range currentBalances {
		balances[currency] = bal
	}

	for i := len(transactions) - 1; i >= 0; i-- {
		tx := transactions[i]
		// undo the crypto effect
		balances[tx.Currency] = balances[tx.Currency].Sub(tx.Amount)

		// undo the USD effect for trades
		if tx.Type == "advanced_trade_fill" || tx.Type == "buy" || tx.Type == "sell" || tx.Type == "trade" {
			balances["USD"] = balances["USD"].Add(tx.NativeAmount)
		}
	}

	// now balances contains genesis state
	genesisBalances := make(map[string]decimal.Decimal)
	for k, v := range balances {
		genesisBalances[k] = v
	}
	genesisValue := calcPortfolioValue(genesisBalances, genesis)
	genesisPrice := getPrice(benchmarkAsset, genesis)

	if !genesisValue.IsPositive() {
		return nil, nil, fmt.Errorf("genesis portfolio value is not positive: %s", genesisValue)
	}

	log.Printf("genesis portfolio value: %s USD", genesisValue)

	// initialize benchmark: buy and hold benchmarkAsset from genesis
	benchmarkQty := genesisValue.Div(genesisPrice)

	// create equity trackers
	strategyEquity := metrics.NewEquity(quantum)
	benchmarkEquity := metrics.NewEquity(quantum)

	// reset to genesis state for forward replay
	for k, v := range genesisBalances {
		balances[k] = v
	}

	// sample at each quantum from genesis to now
	var snapshots []PortfolioSnapshot
	now := clocky.Now()
	txIdx := 0
	totalFees := decimal.Zero
	totalRealized := decimal.Zero
	lots := ds.NewLots(ds.CostBasisMethodLIFO)

	// seed lots with genesis holdings of benchmark asset
	if genesisBalances[benchmarkAsset].IsPositive() {
		lots.Add(genesis, genesisBalances[benchmarkAsset], genesisBalances[benchmarkAsset].Mul(genesisPrice))
	}

	for t := genesis; t <= now; t = t.Add(quantum) {
		// apply transactions up to this time
		for txIdx < len(transactions) && transactions[txIdx].CreatedAt <= t {
			tx := transactions[txIdx]

			// accumulate fees
			totalFees = totalFees.Add(tx.Commission)

			switch tx.Type {
			case "send":
				if tx.Amount.IsPositive() {
					// receiving crypto = deposit
					depositValue := tx.Amount.Mul(getPrice(tx.Currency, tx.CreatedAt))
					// adjust benchmark for deposits of benchmark asset
					if tx.Currency == benchmarkAsset {
						benchmarkQty = benchmarkQty.Add(tx.Amount)
						lots.Add(tx.CreatedAt, tx.Amount, depositValue)
					} else {
						// for other assets, add equivalent benchmark qty
						benchmarkQty = benchmarkQty.Add(depositValue.Div(getPrice(benchmarkAsset, tx.CreatedAt)))
					}
					log.Printf("deposit detected: %s %s (~%s USD)", tx.Amount, tx.Currency, depositValue)
				}
				balances[tx.Currency] = balances[tx.Currency].Add(tx.Amount)

			case "fiat_deposit", "fiat_withdrawal":
				usdAmount := tx.NativeAmount.Abs()
				price := getPrice(benchmarkAsset, tx.CreatedAt)
				if price.IsPositive() {
					if tx.Type == "fiat_deposit" {
						benchmarkQty = benchmarkQty.Add(usdAmount.Div(price))
						balances["USD"] = balances["USD"].Add(usdAmount)
					} else {
						benchmarkQty = benchmarkQty.Sub(usdAmount.Div(price))
						balances["USD"] = balances["USD"].Sub(usdAmount)
					}
				}

			case "advanced_trade_fill", "buy", "sell", "trade":
				balances[tx.Currency] = balances[tx.Currency].Add(tx.Amount)
				balances["USD"] = balances["USD"].Sub(tx.NativeAmount)

				// track lots for realized P/L (only for benchmark asset)
				if tx.Currency == benchmarkAsset {
					if tx.Amount.IsPositive() {
						lots.Add(tx.CreatedAt, tx.Amount, tx.NativeAmount)
					} else {
						sellQty := tx.Amount.Neg()
						price := getPrice(benchmarkAsset, tx.CreatedAt)
						proceeds := sellQty.Mul(price)
						costBasis := lots.Consume(sellQty, price)
						totalRealized = totalRealized.Add(proceeds.Sub(costBasis))
					}
				}

			default:
				if tx.Amount.IsPositive() {
					balances[tx.Currency] = balances[tx.Currency].Add(tx.Amount)
					if tx.Currency == benchmarkAsset {
						benchmarkQty = benchmarkQty.Add(tx.Amount)
						lots.Add(tx.CreatedAt, tx.Amount, decimal.Zero)
					}
				}
			}
			txIdx++
		}

		// calculate current values
		strategyValue := calcPortfolioValue(balances, t)
		benchmarkValue := benchmarkQty.Mul(getPrice(benchmarkAsset, t))

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

	// build holdings list
	var holdings []Holding
	for currency, balance := range balances {
		if balance.IsZero() {
			continue
		}
		price := getPrice(currency, now)
		holdings = append(holdings, Holding{
			Currency: currency,
			Balance:  balance,
			Price:    price,
			Value:    balance.Mul(price),
		})
	}
	// sort by value descending
	sort.Slice(holdings, func(i, j int) bool {
		return holdings[i].Value.Cmp(holdings[j].Value) > 0
	})

	// calculate final metrics
	finalSnapshot := snapshots[len(snapshots)-1]
	startValue := snapshots[0].StrategyValue
	endValue := finalSnapshot.StrategyValue
	benchmarkStart := snapshots[0].BenchmarkValue
	benchmarkEnd := finalSnapshot.BenchmarkValue
	currentPrice := getPrice(benchmarkAsset, now)

	// calculate unrealized P/L for benchmark asset
	currentMarketValue := balances[benchmarkAsset].Mul(currentPrice)
	remainingCostBasis := lots.GetCostBasis(balances[benchmarkAsset], currentPrice)
	totalUnrealized := currentMarketValue.Sub(remainingCostBasis)

	totalReturn := endValue.Sub(startValue).Div(startValue).Float64() * 100
	benchmarkReturn := benchmarkEnd.Sub(benchmarkStart).Div(benchmarkStart).Float64() * 100

	duration := finalSnapshot.Time.Sub(genesis)
	yearsElapsed := float64(duration) / float64(360*clocky.Day)

	result := &ReportMetrics{
		TotalReturn:     totalReturn,
		BenchmarkReturn: benchmarkReturn,
		CurrentValue:    endValue,
		StartValue:      startValue,
		FeesPaid:        totalFees,
		TotalRealized:   totalRealized,
		TotalUnrealized: totalUnrealized,
		Holdings:        holdings,
		BenchmarkAsset:  benchmarkAsset,
		BenchmarkPrice:  currentPrice,
	}

	if yearsElapsed > 0 {
		result.CAGR = (math.Pow(endValue.Float64()/startValue.Float64(), 1/yearsElapsed) - 1) * 100
		result.Sharpe = strategyEquity.Sharpe(0.0487)
		result.MaxDrawdown = strategyEquity.MaxDrawdown() * 100
		result.BenchmarkCAGR = (math.Pow(benchmarkEnd.Float64()/benchmarkStart.Float64(), 1/yearsElapsed) - 1) * 100
		result.BenchmarkSharpe = benchmarkEquity.Sharpe(0.0487)
		result.BenchmarkMaxDD = benchmarkEquity.MaxDrawdown() * 100
	}

	return snapshots, result, nil
}
