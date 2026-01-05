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
	"maps"
	"math"
	"sort"
)

// vipFeeRebate is the fraction of fees rebated for Coinbase VIP (25%).
var vipFeeRebate = decimal.Parse("0.25")

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
	CumulativeFees decimal.Decimal // fees paid up to this point
}

// CashFlow represents a deposit or withdrawal.
type CashFlow struct {
	Time   clocky.Time
	Amount decimal.Decimal // positive = deposit, negative = withdrawal
}

// calculateTWRSeries computes cumulative TWR at each snapshot point.
// Returns TWR-indexed values: startValue * (1 + cumulative_TWR) at each point.
// This shows what the portfolio would be worth from trading alone, without deposit effects.
// feeRebate specifies what fraction of fees to add back (0 = gross fees, 0.25 = VIP rebate, 1 = no fees).
func calculateTWRSeries(snapshots []PortfolioSnapshot, cashFlows []CashFlow, useStrategy bool, feeRebate decimal.Decimal) []float64 {
	if len(snapshots) < 1 {
		return nil
	}

	// Build a map of cash flows by time for quick lookup
	cfByTime := make(map[clocky.Time]decimal.Decimal)
	for _, cf := range cashFlows {
		cfByTime[cf.Time] = cfByTime[cf.Time].Add(cf.Amount)
	}

	// Get starting value (with fee adjustment for strategy)
	var startVal decimal.Decimal
	if useStrategy {
		startVal = snapshots[0].StrategyValue.Add(snapshots[0].CumulativeFees.Mul(feeRebate))
	} else {
		startVal = snapshots[0].BenchmarkValue
	}

	result := make([]float64, len(snapshots))
	result[0] = startVal.Float64()

	// Calculate cumulative TWR at each point
	cumulativeTWR := decimal.One
	for i := 1; i < len(snapshots); i++ {
		var prevVal, currVal decimal.Decimal
		if useStrategy {
			// Add back the rebated portion of fees
			prevVal = snapshots[i-1].StrategyValue.Add(snapshots[i-1].CumulativeFees.Mul(feeRebate))
			currVal = snapshots[i].StrategyValue.Add(snapshots[i].CumulativeFees.Mul(feeRebate))
		} else {
			prevVal = snapshots[i-1].BenchmarkValue
			currVal = snapshots[i].BenchmarkValue
		}

		// Subtract any cash flows that happened in this period
		periodCashFlow := decimal.Zero
		for t, amt := range cfByTime {
			if t > snapshots[i-1].Time && t <= snapshots[i].Time {
				periodCashFlow = periodCashFlow.Add(amt)
			}
		}

		// Calculate sub-period return and accumulate
		adjustedEnd := currVal.Sub(periodCashFlow)
		if prevVal.IsPositive() {
			subPeriodReturn := adjustedEnd.Div(prevVal)
			cumulativeTWR = cumulativeTWR.Mul(subPeriodReturn)
		}

		// TWR-indexed value: what would portfolio be worth from trading alone
		result[i] = startVal.Mul(cumulativeTWR).Float64()
	}

	return result
}

// calculateTWR computes total Time-Weighted Return from snapshots and cash flows.
func calculateTWR(snapshots []PortfolioSnapshot, cashFlows []CashFlow, useStrategy bool, feeRebate decimal.Decimal) float64 {
	series := calculateTWRSeries(snapshots, cashFlows, useStrategy, feeRebate)
	if len(series) < 2 {
		return 0
	}
	return (series[len(series)-1] / series[0]) - 1.0
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
// Returns snapshots, cash flows (for TWR chart), and metrics.
func calculateMetrics(
	client *coinbase.Client,
	benchmarkAsset string,
	genesis clocky.Time,
	quantum clocky.Duration,
) ([]PortfolioSnapshot, []CashFlow, *ReportMetrics, error) {
	// get all accounts and sync transactions for each
	accounts, err := client.GetAccounts()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("getting accounts: %w", err)
	}

	// sync transactions for all currencies with non-zero balance
	currencies := make(map[string]bool)
	currentBalances := make(map[string]decimal.Decimal)
	for _, acc := range accounts {
		bal := acc.AvailableBalance.Value
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
		return nil, nil, nil, err
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
		return nil, nil, nil, fmt.Errorf("no price data available for benchmark asset %s-USD", benchmarkAsset)
	}

	// helper to get price for any currency at a time
	getPrice := func(currency string, t clocky.Time) decimal.Decimal {
		if currency == "USD" {
			return decimal.One
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
	maps.Copy(balances, currentBalances)

	for i := len(transactions) - 1; i >= 0; i-- {
		tx := transactions[i]
		// undo the crypto effect
		balances[tx.Currency] = balances[tx.Currency].Sub(tx.Amount)

		// undo the USD effect for trades (including fees)
		if tx.Type == "advanced_trade_fill" || tx.Type == "buy" || tx.Type == "sell" || tx.Type == "trade" {
			balances["USD"] = balances["USD"].Add(tx.NativeAmount)
			if tx.Currency != "USD" {
				balances["USD"] = balances["USD"].Add(tx.Commission)
			}
		}
	}

	// now balances contains genesis state
	genesisBalances := make(map[string]decimal.Decimal)
	maps.Copy(genesisBalances, balances)
	genesisValue := calcPortfolioValue(genesisBalances, genesis)
	genesisPrice := getPrice(benchmarkAsset, genesis)

	if !genesisValue.IsPositive() {
		return nil, nil, nil, fmt.Errorf("genesis portfolio value is not positive: %s", genesisValue)
	}

	log.Printf("genesis portfolio value: %s USD", genesisValue)

	// initialize benchmark: buy and hold benchmarkAsset from genesis
	benchmarkQty := genesisValue.Div(genesisPrice)

	// reset to genesis state for forward replay
	maps.Copy(balances, genesisBalances)

	// sample at each quantum from genesis to now
	var snapshots []PortfolioSnapshot
	var cashFlows []CashFlow // track deposits/withdrawals for TWR
	now := clocky.Now()
	txIdx := 0
	totalFees := decimal.Zero
	totalRealized := decimal.Zero
	lots := ds.NewLots(ds.CostBasisMethodLIFO)

	// seed lots with genesis holdings of benchmark asset
	if genesisBalances[benchmarkAsset].IsPositive() {
		lots.Add(genesis, genesisBalances[benchmarkAsset],
			genesisBalances[benchmarkAsset].Mul(genesisPrice))
	}

	for t := genesis; t <= now; t = t.Add(quantum) {
		// apply transactions up to this time
		for txIdx < len(transactions) && transactions[txIdx].CreatedAt <= t {
			tx := transactions[txIdx]

			// accumulate fees (only from crypto records - USD records duplicate the same fees)
			if tx.Currency != "USD" {
				totalFees = totalFees.Add(tx.Commission)
			}

			switch tx.Type {
			case "send":
				depositValue := tx.Amount.Mul(getPrice(tx.Currency, tx.CreatedAt))
				price := getPrice(benchmarkAsset, tx.CreatedAt)
				if tx.Amount.IsPositive() {
					// receiving crypto = deposit
					if tx.Currency == benchmarkAsset {
						benchmarkQty = benchmarkQty.Add(tx.Amount)
						lots.Add(tx.CreatedAt, tx.Amount, depositValue)
					} else if price.IsPositive() {
						benchmarkQty = benchmarkQty.Add(depositValue.Div(price))
					}
					cashFlows = append(cashFlows, CashFlow{tx.CreatedAt, depositValue})
					log.Printf("send deposit: %s %s (~%s USD)", tx.Amount, tx.Currency, depositValue)
				} else if tx.Amount.IsNegative() {
					// sending crypto = withdrawal
					withdrawalValue := depositValue.Neg()
					if tx.Currency == benchmarkAsset {
						benchmarkQty = benchmarkQty.Sub(tx.Amount.Neg())
					} else if price.IsPositive() {
						benchmarkQty = benchmarkQty.Sub(withdrawalValue.Div(price))
					}
					cashFlows = append(cashFlows, CashFlow{tx.CreatedAt, withdrawalValue.Neg()})
					log.Printf("send withdrawal: %s %s (~%s USD)", tx.Amount.Neg(), tx.Currency, withdrawalValue)
				}
				balances[tx.Currency] = balances[tx.Currency].Add(tx.Amount)

			case "fiat_deposit", "fiat_withdrawal":
				usdAmount := tx.NativeAmount.Abs()
				price := getPrice(benchmarkAsset, tx.CreatedAt)
				if price.IsPositive() {
					if tx.Type == "fiat_deposit" {
						benchmarkQty = benchmarkQty.Add(usdAmount.Div(price))
						balances["USD"] = balances["USD"].Add(usdAmount)
						cashFlows = append(cashFlows, CashFlow{tx.CreatedAt, usdAmount})
						log.Printf("fiat deposit: %s USD", usdAmount)
					} else {
						benchmarkQty = benchmarkQty.Sub(usdAmount.Div(price))
						balances["USD"] = balances["USD"].Sub(usdAmount)
						cashFlows = append(cashFlows, CashFlow{tx.CreatedAt, usdAmount.Neg()})
						log.Printf("fiat withdrawal: %s USD", usdAmount)
					}
				}

			case "tx":
				// Generic transaction - can be deposit/withdrawal of any currency
				usdValue := tx.NativeAmount.Abs()
				price := getPrice(benchmarkAsset, tx.CreatedAt)
				if price.IsPositive() && !usdValue.IsZero() {
					if tx.Amount.IsPositive() {
						// Deposit
						benchmarkQty = benchmarkQty.Add(usdValue.Div(price))
						cashFlows = append(cashFlows, CashFlow{tx.CreatedAt, usdValue})
						log.Printf("tx deposit: %s %s (~%s USD)", tx.Amount, tx.Currency, usdValue)
					} else if tx.Amount.IsNegative() {
						// Withdrawal
						benchmarkQty = benchmarkQty.Sub(usdValue.Div(price))
						cashFlows = append(cashFlows, CashFlow{tx.CreatedAt, usdValue.Neg()})
						log.Printf("tx withdrawal: %s %s (~%s USD)", tx.Amount.Neg(), tx.Currency, usdValue)
					}
				}
				balances[tx.Currency] = balances[tx.Currency].Add(tx.Amount)

			case "advanced_trade_fill", "buy", "sell", "trade":
				balances[tx.Currency] = balances[tx.Currency].Add(tx.Amount)
				balances["USD"] = balances["USD"].Sub(tx.NativeAmount)
				// Fees are charged separately from native_amount and reduce USD
				// (only crypto-side records have commission, USD-side records don't)
				if tx.Currency != "USD" {
					balances["USD"] = balances["USD"].Sub(tx.Commission)
				}

				// track lots for realized P/L (only for benchmark asset)
				// Cost basis includes fees paid on purchase
				// Proceeds are actual native_amount received minus fees on sale
				if tx.Currency == benchmarkAsset {
					if tx.Amount.IsPositive() {
						// Buy: cost basis = native_amount + commission
						costWithFee := tx.NativeAmount.Add(tx.Commission)
						lots.Add(tx.CreatedAt, tx.Amount, costWithFee)
					} else {
						// Sell: proceeds = native_amount - commission
						sellQty := tx.Amount.Neg()
						proceeds := tx.NativeAmount.Abs().Sub(tx.Commission)
						price := getPrice(benchmarkAsset, tx.CreatedAt)
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

		if strategyValue.IsPositive() && benchmarkValue.IsPositive() {
			snapshots = append(snapshots, PortfolioSnapshot{
				Time:           t,
				StrategyValue:  strategyValue,
				BenchmarkValue: benchmarkValue,
				CumulativeFees: totalFees,
			})
		}
	}

	if len(snapshots) < 2 {
		return nil, nil, nil, fmt.Errorf("not enough data points for metrics calculation")
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
	currentPrice := getPrice(benchmarkAsset, now)

	// calculate unrealized P/L for benchmark asset
	currentMarketValue := balances[benchmarkAsset].Mul(currentPrice)
	remainingCostBasis := lots.GetCostBasis(balances[benchmarkAsset], currentPrice)
	totalUnrealized := currentMarketValue.Sub(remainingCostBasis)

	// Calculate TWR series for Sharpe/MaxDD (removes deposit distortion)
	// Use 0.25 rebate to reflect VIP 25% fee rebate (i.e., only 75% of fees are effectively paid)
	strategySeries := calculateTWRSeries(snapshots, cashFlows, true, vipFeeRebate)
	benchmarkSeries := calculateTWRSeries(snapshots, cashFlows, false, decimal.Zero)

	// Use Time-Weighted Return to eliminate impact of deposits/withdrawals
	totalReturn := (strategySeries[len(strategySeries)-1]/strategySeries[0] - 1) * 100
	benchmarkReturn := (benchmarkSeries[len(benchmarkSeries)-1]/benchmarkSeries[0] - 1) * 100

	log.Printf("TWR calculation: %d cash flows, strategy=%.2f%% (with 25%% rebate), benchmark=%.2f%%",
		len(cashFlows), totalReturn, benchmarkReturn)

	duration := finalSnapshot.Time.Sub(genesis)
	yearsElapsed := float64(duration) / float64(360*clocky.Day)

	// Feed TWR-indexed values to equity trackers for Sharpe/MaxDD
	strategyEquity := metrics.NewEquity(quantum)
	benchmarkEquity := metrics.NewEquity(quantum)
	for i, s := range snapshots {
		if strategyEquity.ShouldSample(s.Time) {
			strategyEquity.Sample(s.Time, decimal.FromFloat64(strategySeries[i]))
			benchmarkEquity.Sample(s.Time, decimal.FromFloat64(benchmarkSeries[i]))
		}
	}

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
		// CAGR from TWR: (1 + TWR)^(1/years) - 1
		strategyTWR := totalReturn / 100 // convert back to decimal
		benchmarkTWR := benchmarkReturn / 100
		result.CAGR = (math.Pow(1+strategyTWR, 1/yearsElapsed) - 1) * 100
		result.Sharpe = strategyEquity.Sharpe(0.0487)
		result.MaxDrawdown = strategyEquity.MaxDrawdown() * 100
		result.BenchmarkCAGR = (math.Pow(1+benchmarkTWR, 1/yearsElapsed) - 1) * 100
		result.BenchmarkSharpe = benchmarkEquity.Sharpe(0.0487)
		result.BenchmarkMaxDD = benchmarkEquity.MaxDrawdown() * 100
	}

	return snapshots, cashFlows, result, nil
}
