package main

import (
	"bytes"
	"database/sql"
	"dropbear/broker/coinbase"
	"dropbear/clocky"
	"dropbear/db"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"dropbear/teddy/metrics"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

//go:embed static
var staticFiles embed.FS

// vipFeeRebateRate is the Coinbase VIP fee rebate (25%).
// The "Fees Paid" metric shows net fees after rebate.
// Transaction log shows gross fees; rebate sweeps appear as separate transactions.
var vipFeeRebateRate = decimal.Parse("0.75") // 1 - 0.25 rebate

// ChartData is the JSON structure for the chart.
type ChartData struct {
	Strategy  []ChartPoint `json:"strategy"`
	Benchmark []ChartPoint `json:"benchmark"`
	// Fee-free versions for toggle comparison
	StrategyNoFees  []ChartPoint `json:"strategyNoFees"`
	BenchmarkNoFees []ChartPoint `json:"benchmarkNoFees"`
	Metrics         struct {
		TotalReturn     float64 `json:"totalReturn"`
		CAGR            float64 `json:"cagr"`
		Sharpe          float64 `json:"sharpe"`
		MaxDrawdown     float64 `json:"maxDrawdown"`
		BenchmarkReturn float64 `json:"benchmarkReturn"`
	} `json:"metrics"`
	// Fee-free metrics
	MetricsNoFees struct {
		TotalReturn     float64 `json:"totalReturn"`
		CAGR            float64 `json:"cagr"`
		Sharpe          float64 `json:"sharpe"`
		MaxDrawdown     float64 `json:"maxDrawdown"`
		BenchmarkReturn float64 `json:"benchmarkReturn"`
	} `json:"metricsNoFees"`
	FeesPaid float64 `json:"feesPaid"`
}

// ChartPoint is a single data point for the chart.
// Time is Unix seconds for TradingView Lightweight Charts.
type ChartPoint struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}

// TransactionRow is data for a transaction table row.
type TransactionRow struct {
	Date       string
	Type       string
	Side       string // Buy or Sell
	Currency   string // asset currency (ETH, BTC, etc.)
	Amount     string
	FillPrice  string // price per unit
	Notional   string
	CostBasis  string // LIFO cost basis for sells
	Fee        string // commission paid
	ProfitLoss string // LIFO profit/loss for sells
	Status     string
}

// generateReport creates the static HTML report files.
func generateReport(client *coinbase.Client, asset, algorithm string, genesis clocky.Time, quantum clocky.Duration, outputDir string) error {
	log.Printf("generating report for %s since %s with quantum %s", asset, genesis, quantum)

	// calculate metrics and get snapshots
	snapshots, cashFlows, metrics, err := calculateMetrics(client, asset, genesis, quantum)
	if err != nil {
		return fmt.Errorf("calculating metrics: %w", err)
	}
	log.Printf("calculated metrics: return=%.2f%%, benchmark=%.2f%%, sharpe=%.2f",
		metrics.TotalReturn, metrics.BenchmarkReturn, metrics.Sharpe)

	// get deposit address
	depositAddress, networkName := getDepositAddress(client, asset)
	log.Printf("deposit address: %s (%s)", depositAddress, networkName)

	// fetch transactions for display with LIFO profit/loss
	transactions, err := fetchTransactionsForDisplay(client, asset, genesis)
	if err != nil {
		log.Printf("warning: failed to fetch transactions for display: %v", err)
	}

	// calculate duration since genesis
	duration := clocky.Now().Sub(genesis)

	// build chart data JSON using TWR-indexed values (removes deposit jumps)
	chartData := buildChartData(snapshots, cashFlows, metrics, quantum)
	chartJSON, err := json.MarshalIndent(chartData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling chart data: %w", err)
	}

	// generate HTML
	htmlContent := generateHTML(asset, algorithm, duration, metrics, depositAddress, networkName, transactions)

	// read static files
	cssContent, err := staticFiles.ReadFile("static/style.css")
	if err != nil {
		return fmt.Errorf("reading style.css: %w", err)
	}
	jsContent, err := staticFiles.ReadFile("static/chart.js")
	if err != nil {
		return fmt.Errorf("reading chart.js: %w", err)
	}

	// write files atomically
	files := map[string][]byte{
		"index.html": []byte(htmlContent),
		"style.css":  cssContent,
		"chart.js":   jsContent,
		"data.json":  chartJSON,
	}

	for name, content := range files {
		path := filepath.Join(outputDir, name)
		if err := writeFileAtomic(path, content); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
		log.Printf("wrote %s (%d bytes)", path, len(content))
	}

	return nil
}

// getDepositAddress fetches the deposit address for the asset.
func getDepositAddress(client *coinbase.Client, asset string) (address, network string) {
	accounts, err := client.GetAccounts()
	if err != nil {
		log.Printf("warning: failed to get v2 account for %s: %v", asset, err)
		return "unavailable", ""
	}
	var accountID string
	for _, account := range accounts {
		if account.Currency == asset {
			accountID = account.UUID
			break
		}
	}
	if accountID == "" {
		loggy.Fatalf("no coinbase account found for asset %s", asset)
	}

	addresses, err := client.GetDepositAddresses(accountID)
	if err != nil {
		log.Printf("warning: failed to get deposit addresses: %v", err)
		return "unavailable", ""
	}

	if len(addresses) == 0 {
		// try to create one
		addr, err := client.CreateDepositAddress(accountID)
		if err != nil {
			log.Printf("warning: failed to create deposit address: %v", err)
			return "unavailable", ""
		}
		return addr.Address, addr.Network
	}

	return addresses[0].Address, addresses[0].Network
}

// fetchTransactionsForDisplay fetches transactions for the HTML table.
// Includes LIFO profit/loss calculation for sell transactions of the benchmark asset.
func fetchTransactionsForDisplay(_ *coinbase.Client, benchmarkAsset string, genesis clocky.Time) ([]TransactionRow, error) {
	database := db.Get()

	// First pass: build lots by replaying transactions in chronological order
	// We need to process in ASC order to correctly consume lots for P/L calculation
	lots := ds.NewLots(ds.CostBasisMethodLIFO)
	plMap := make(map[string]struct { // map transaction ID -> P/L info
		CostBasis  decimal.Decimal
		ProfitLoss decimal.Decimal
	})

	// Query all benchmark asset transactions to build lots
	lotsRows, err := database.Query(`
		SELECT id, type, created_at, amount, native_amount,
		       COALESCE(fill_side, ''), COALESCE(fill_price, ''), COALESCE(fill_commission, '')
		FROM coinbase_transactions
		WHERE currency = :currency AND status = 'completed'
		ORDER BY created_at ASC
	`, sql.Named("currency", benchmarkAsset))
	if err != nil {
		log.Printf("warning: failed to query lots: %v", err)
	} else {
		defer lotsRows.Close()
		for lotsRows.Next() {
			var id, txType, amountStr, nativeStr, fillSide, fillPriceStr, commissionStr string
			var createdAt int64
			if err := lotsRows.Scan(&id, &txType, &createdAt, &amountStr, &nativeStr, &fillSide, &fillPriceStr, &commissionStr); err != nil {
				continue
			}
			t := clocky.Time(createdAt)
			amount := decimal.Parse(amountStr)
			native := decimal.Parse(nativeStr)
			var commission, fillPrice decimal.Decimal
			if commissionStr != "" {
				commission = decimal.Parse(commissionStr)
			}
			if fillPriceStr != "" {
				fillPrice = decimal.Parse(fillPriceStr)
			}

			switch txType {
			case "advanced_trade_fill", "buy", "sell", "trade":
				if fillSide == "buy" && amount.IsPositive() {
					lots.Add(t, amount, native.Abs())
				} else if fillSide == "sell" && amount.IsNegative() && !fillPrice.IsZero() {
					sellQty := amount.Neg()
					proceeds := native.Abs()
					costBasis := lots.Consume(sellQty, fillPrice)
					profitLoss := proceeds.Sub(costBasis).Sub(commission)
					plMap[id] = struct {
						CostBasis  decimal.Decimal
						ProfitLoss decimal.Decimal
					}{costBasis, profitLoss}
				}
			case "send":
				if amount.IsPositive() {
					// deposit - add at zero cost basis (or use native if available)
					lots.Add(t, amount, native.Abs())
				}
			}
		}
	}

	// Second pass: fetch all transactions for display (DESC order)
	// Exclude USD records for advanced_trade_fill (they mirror crypto trades)
	// But include USD records for tx/fiat_deposit/fiat_withdrawal (actual deposits)
	rows, err := database.Query(`
		SELECT id, type, status, created_at, amount, native_amount, currency,
		       COALESCE(fill_side, ''), COALESCE(fill_price, ''), COALESCE(fill_commission, '')
		FROM coinbase_transactions
		WHERE created_at >= :genesis
		  AND NOT (currency = 'USD' AND type = 'advanced_trade_fill')
		ORDER BY created_at DESC
		LIMIT 10000
	`, sql.Named("genesis", int64(genesis)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TransactionRow
	for rows.Next() {
		var id, txType, status, amountStr, nativeStr, currency, fillSide, fillPriceStr, commissionStr string
		var createdAt int64
		if err := rows.Scan(&id, &txType, &status, &createdAt, &amountStr, &nativeStr, &currency, &fillSide, &fillPriceStr, &commissionStr); err != nil {
			continue
		}

		t := clocky.Time(createdAt)
		amount := decimal.Parse(amountStr)
		native := decimal.Parse(nativeStr)
		var commission, fillPrice decimal.Decimal
		if commissionStr != "" {
			commission = decimal.Parse(commissionStr)
		}
		if fillPriceStr != "" {
			fillPrice = decimal.Parse(fillPriceStr)
		}

		// Determine display type - tx type needs special handling based on amount sign
		displayType := formatTxType(txType)
		if txType == "tx" {
			if amount.IsPositive() {
				// Small USD transfers (<$500) from primary are rebate sweeps
				if currency == "USD" && native.Abs().Cmp(decimal.Parse("500")) < 0 {
					displayType = "Rebate"
				} else {
					displayType = "Deposit"
				}
			} else {
				displayType = "Withdrawal"
			}
		}

		// For trades, flip notional sign (sells positive, buys negative)
		// For deposits/withdrawals, keep native sign (deposits positive, withdrawals negative)
		notional := native
		if txType == "advanced_trade_fill" || txType == "buy" || txType == "sell" || txType == "trade" {
			notional = native.Neg()
		}

		row := TransactionRow{
			Date:     t.String()[:19], // trim microseconds
			Type:     displayType,
			Currency: currency,
			Amount:   formatAmount(amount, currency, 8),
			Notional: formatAmount(notional, "USD", 2),
			Status:   status,
		}

		// show side if present
		switch fillSide {
		case "buy":
			row.Side = "Buy"
		case "sell":
			row.Side = "Sell"
		}

		// show fill price if present
		if !fillPrice.IsZero() {
			row.FillPrice = formatAmount(fillPrice, "USD", 8)
		}

		// show fee if present
		if !commission.IsZero() {
			row.Fee = formatAmount(commission.Neg(), "USD", 8) // show as negative (cost)
		}

		// use pre-calculated P/L for benchmark asset sells
		if pl, ok := plMap[id]; ok {
			row.CostBasis = formatAmount(pl.CostBasis, "USD", 8)
			row.ProfitLoss = formatAmount(pl.ProfitLoss, "USD", 8)
		}

		result = append(result, row)
	}
	return result, rows.Err()
}

// formatTxType formats transaction type for display.
func formatTxType(t string) string {
	switch t {
	case "advanced_trade_fill":
		return "Trade"
	case "send":
		return "Transfer"
	case "fiat_deposit":
		return "Deposit"
	case "fiat_withdrawal":
		return "Withdrawal"
	default:
		return strings.Title(strings.ReplaceAll(t, "_", " "))
	}
}

// formatAmount formats a decimal amount for display.
func formatAmount(d decimal.Decimal, currency string, places int) string {
	if currency == "USD" {
		if d.IsPositive() {
			return "$" + d.FormatThousand(places)
		} else {
			return "-$" + d.Abs().FormatThousand(places)
		}
	} else {
		return d.FormatThousand(places) + " " + currency
	}
}

// buildChartData creates the chart data structure using TWR-indexed values.
// This removes the visual effect of deposits/withdrawals from the chart,
// showing only the trading performance (what the portfolio would be worth
// if there were no external cash flows).
// Also includes fee-free versions for toggle comparison.
func buildChartData(snapshots []PortfolioSnapshot, cashFlows []CashFlow, m *ReportMetrics, quantum clocky.Duration) ChartData {
	data := ChartData{}
	data.Metrics.TotalReturn = m.TotalReturn
	data.Metrics.CAGR = m.CAGR
	data.Metrics.Sharpe = m.Sharpe
	data.Metrics.MaxDrawdown = m.MaxDrawdown
	data.Metrics.BenchmarkReturn = m.BenchmarkReturn
	data.FeesPaid = m.FeesPaid.Mul(vipFeeRebateRate).Float64() // net fees after 25% rebate

	// Calculate TWR-indexed values for chart (removes deposit jumps)
	// Main series uses VIP rebate (25% of fees added back)
	strategySeries := calculateTWRSeries(snapshots, cashFlows, true, vipFeeRebate)
	benchmarkSeries := calculateTWRSeries(snapshots, cashFlows, false, decimal.Zero)

	// Calculate fee-free series (what performance would be without fees)
	strategyNoFeesSeries := calculateTWRSeries(snapshots, cashFlows, true, decimal.One)
	benchmarkNoFeesSeries := calculateTWRSeries(snapshots, cashFlows, false, decimal.Zero)

	for i, s := range snapshots {
		unixSec := s.Time.Unix()
		data.Strategy = append(data.Strategy, ChartPoint{
			Time:  unixSec,
			Value: strategySeries[i],
		})
		data.Benchmark = append(data.Benchmark, ChartPoint{
			Time:  unixSec,
			Value: benchmarkSeries[i],
		})
		data.StrategyNoFees = append(data.StrategyNoFees, ChartPoint{
			Time:  unixSec,
			Value: strategyNoFeesSeries[i],
		})
		data.BenchmarkNoFees = append(data.BenchmarkNoFees, ChartPoint{
			Time:  unixSec,
			Value: benchmarkNoFeesSeries[i],
		})
	}

	// Calculate fee-free metrics
	if len(strategyNoFeesSeries) >= 2 {
		noFeesReturn := (strategyNoFeesSeries[len(strategyNoFeesSeries)-1]/strategyNoFeesSeries[0] - 1) * 100
		benchNoFeesReturn := (benchmarkNoFeesSeries[len(benchmarkNoFeesSeries)-1]/benchmarkNoFeesSeries[0] - 1) * 100
		data.MetricsNoFees.TotalReturn = noFeesReturn
		data.MetricsNoFees.BenchmarkReturn = benchNoFeesReturn

		// Calculate CAGR for fee-free
		duration := snapshots[len(snapshots)-1].Time.Sub(snapshots[0].Time)
		yearsElapsed := float64(duration) / float64(360*clocky.Day)
		if yearsElapsed > 0 {
			data.MetricsNoFees.CAGR = (math.Pow(1+noFeesReturn/100, 1/yearsElapsed) - 1) * 100

			// Calculate Sharpe/MaxDD for fee-free using equity tracker
			noFeesEquity := metrics.NewEquity(quantum)
			for i, s := range snapshots {
				if noFeesEquity.ShouldSample(s.Time) {
					noFeesEquity.Sample(s.Time, decimal.FromFloat64(strategyNoFeesSeries[i]))
				}
			}
			data.MetricsNoFees.Sharpe = noFeesEquity.Sharpe(0.0487)
			data.MetricsNoFees.MaxDrawdown = noFeesEquity.MaxDrawdown() * 100
		}
	}

	return data
}

// generateHTML creates the HTML content.
func generateHTML(asset, algorithm string, duration clocky.Duration, m *ReportMetrics, depositAddr, network string, transactions []TransactionRow) string {
	var buf bytes.Buffer

	// calculate vs benchmark (alpha)
	alpha := m.TotalReturn - m.BenchmarkReturn

	buf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>`)
	buf.WriteString(html.EscapeString(asset))
	buf.WriteString(`-USD Spot Trading</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <div class="container">
        <h1>`)
	buf.WriteString(html.EscapeString(asset))
	buf.WriteString(`-USD SPOT TRADING</h1>
        <p class="tagline">Justine Street `)
	buf.WriteString(html.EscapeString(strings.ToUpper(algorithm)))
	buf.WriteString(`</p>

        <section class="metrics">
            <div class="metric">
                <span class="label">Duration</span>
                <span class="value">`)
	buf.WriteString(formatDuration(duration))
	buf.WriteString(`</span>
            </div>
            <div class="metric fee-toggle" id="fee-toggle" title="Click to toggle fees on/off. Net of 25% VIP rebate.">
                <span class="label" id="fee-label">Net Fees</span>
                <span class="value negative" id="fee-value">-$`)
	buf.WriteString(m.FeesPaid.Mul(vipFeeRebateRate).Format(2))
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label">Realized P/L</span>
                <span class="value`)
	if m.TotalRealized.IsPositive() {
		buf.WriteString(" positive")
	} else if m.TotalRealized.IsNegative() {
		buf.WriteString(" negative")
	}
	buf.WriteString(`">`)
	if m.TotalRealized.IsPositive() {
		buf.WriteString("+")
	}
	if m.TotalRealized.IsNegative() {
		buf.WriteString("-")
	}
	buf.WriteString("$")
	buf.WriteString(m.TotalRealized.Abs().Format(2))
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label">Unrealized P/L</span>
                <span class="value`)
	if m.TotalUnrealized.IsPositive() {
		buf.WriteString(" positive")
	} else if m.TotalUnrealized.IsNegative() {
		buf.WriteString(" negative")
	}
	buf.WriteString(`">`)
	if m.TotalUnrealized.IsPositive() {
		buf.WriteString("+")
	}
	if m.TotalUnrealized.IsNegative() {
		buf.WriteString("-")
	}
	buf.WriteString("$")
	buf.WriteString(m.TotalUnrealized.Abs().Format(2))
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label">Total Return</span>
                <span id="metric-return" class="value`)
	if m.TotalReturn >= 0 {
		buf.WriteString(" positive")
	} else {
		buf.WriteString(" negative")
	}
	buf.WriteString(`">`)
	buf.WriteString(fmt.Sprintf("%+.1f%%", m.TotalReturn))
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label" title="Compounding Annualized Growth Rate">CAGR</span>
                <span id="metric-cagr" class="value`)
	if m.CAGR >= 0 {
		buf.WriteString(" positive")
	} else {
		buf.WriteString(" negative")
	}
	buf.WriteString(`">`)
	buf.WriteString(fmt.Sprintf("%.1f%%", m.CAGR))
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label">Sharpe Ratio</span>
                <span id="metric-sharpe" class="value">`)
	buf.WriteString(fmt.Sprintf("%.2f", m.Sharpe))
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label">Max Drawdown</span>
                <span id="metric-maxdd" class="value negative">`)
	buf.WriteString(fmt.Sprintf("-%.1f%%", m.MaxDrawdown))
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label">vs Benchmark</span>
                <span id="metric-alpha" class="value`)
	if alpha >= 0 {
		buf.WriteString(" positive")
	} else {
		buf.WriteString(" negative")
	}
	buf.WriteString(`">`)
	buf.WriteString(fmt.Sprintf("%+.1f%%", alpha))
	buf.WriteString(`</span>
            </div>
        </section>

        <section class="chart-section">
            <div id="chart"></div>
        </section>

        <section class="portfolio">
            <h2>ASSETS UNDER MANAGEMENT</h2>
            <div class="portfolio-value">$`)
	buf.WriteString(m.CurrentValue.FormatThousand(2))
	buf.WriteString(`</div>
            <div class="holdings">`)
	for _, h := range m.Holdings {
		// hide assets worth less than $1
		if h.Value.Cmp(decimal.One) < 0 {
			continue
		}
		buf.WriteString(`
                <span class="holding">`)
		if h.Currency == "USD" {
			buf.WriteString("$")
			buf.WriteString(h.Balance.FormatThousand(8))
			buf.WriteString(" USD")
		} else {
			buf.WriteString(h.Balance.String())
			buf.WriteString(" ")
			buf.WriteString(html.EscapeString(h.Currency))
			// buf.WriteString(" @ $")
			// buf.WriteString(h.Price.FormatThousand(2))
		}
		buf.WriteString(`</span>`)
	}
	buf.WriteString(`
            </div>
        </section>

        <section class="deposit">
            <h2>PORTFOLIO ADDRESS</h2>
            <p class="network">`)
	if network != "" {
		buf.WriteString(html.EscapeString(network))
	} else {
		buf.WriteString(html.EscapeString(asset))
	}
	buf.WriteString(` Network</p>
            <code class="address">`)
	buf.WriteString(html.EscapeString(depositAddr))
	buf.WriteString(`</code>
            <p class="disclaimer">
                The above address is owned by <a href="https://justine.lol/">Justine Alexandra Roberts Tunney</a>. By sending `)
	buf.WriteString(html.EscapeString(asset))
	buf.WriteString(` to this address, you get the benefit of entertainment in seeing the crypto show up on this page and being traded, and the satisfaction of knowing that you're supporting her work in fields such as open source development, artificial intelligence, finance, and grassroots activism. <strong>This is not an investment.</strong> She's not a registered anything. You are giving her the crypto. She has full autonomy over how she uses her crypto but promises you'll at least get to see it in action as part of this live trading portfolio for a short time. She has no way of knowing who sent her the crypto. You should also consider the tax implications of sharing cryptography.
            </p>
        </section>

        <section class="benchmark">
            <h2>BENCHMARK (BUY &amp; HOLD)</h2>
            <div class="metrics horizontal">
                <div class="metric">
                    <span class="label">Return</span>
                    <span class="value">`)
	buf.WriteString(fmt.Sprintf("%+.1f%%", m.BenchmarkReturn))
	buf.WriteString(`</span>
                </div>
                <div class="metric">
                    <span class="label">Sharpe</span>
                    <span class="value">`)
	buf.WriteString(fmt.Sprintf("%.2f", m.BenchmarkSharpe))
	buf.WriteString(`</span>
                </div>
                <div class="metric">
                    <span class="label">Max DD</span>
                    <span class="value">`)
	buf.WriteString(fmt.Sprintf("-%.1f%%", m.BenchmarkMaxDD))
	buf.WriteString(`</span>
                </div>
            </div>
        </section>

        <section class="transactions">
            <h2>RECENT TRANSACTION HISTORY</h2>
            <table>
                <thead>
                    <tr>
                        <th>Date</th>
                        <th>Type</th>
                        <th>Side</th>
                        <th>Status</th>
                        <th class="num">Amount</th>
                        <th class="num">Price</th>
                        <th class="num">Notional</th>
                        <th class="num">Cost Basis</th>
                        <th class="num">Fee</th>
                        <th class="num">P/L</th>
                    </tr>
                </thead>
                <tbody>
`)

	for _, tx := range transactions {
		buf.WriteString("                    <tr>\n")
		buf.WriteString("                        <td>")
		buf.WriteString(html.EscapeString(tx.Date))
		buf.WriteString("</td>\n")
		buf.WriteString("                        <td>")
		buf.WriteString(html.EscapeString(tx.Type))
		buf.WriteString("</td>\n")
		buf.WriteString("                        <td>")
		buf.WriteString(html.EscapeString(tx.Side))
		buf.WriteString("</td>\n")
		buf.WriteString("                        <td>")
		buf.WriteString(html.EscapeString(tx.Status))
		buf.WriteString("</td>\n")
		buf.WriteString("                        <td class=\"num\">")
		buf.WriteString(html.EscapeString(tx.Amount))
		buf.WriteString("</td>\n")
		buf.WriteString("                        <td class=\"num\">")
		buf.WriteString(html.EscapeString(tx.FillPrice))
		buf.WriteString("</td>\n")
		buf.WriteString("                        <td class=\"num\">")
		buf.WriteString(html.EscapeString(tx.Notional))
		buf.WriteString("</td>\n")
		buf.WriteString("                        <td class=\"num\">")
		buf.WriteString(html.EscapeString(tx.CostBasis))
		buf.WriteString("</td>\n")

		// fee column
		feeClass := "fee num"
		buf.WriteString("                        <td class=\"")
		buf.WriteString(feeClass)
		buf.WriteString("\">")
		buf.WriteString(html.EscapeString(tx.Fee))
		buf.WriteString("</td>\n")

		// P/L column with positive/negative coloring
		var plClass string
		if strings.HasPrefix(tx.ProfitLoss, "-") {
			plClass = "num negative"
		} else {
			plClass = "num positive"
		}
		buf.WriteString("                        <td class=\"")
		buf.WriteString(plClass)
		buf.WriteString("\">")
		buf.WriteString(html.EscapeString(tx.ProfitLoss))
		buf.WriteString("</td>\n")
		buf.WriteString("                    </tr>\n")
	}

	if len(transactions) == 0 {
		buf.WriteString("                    <tr><td colspan=\"10\" class=\"no-data\">No transactions since genesis</td></tr>\n")
	}

	buf.WriteString(`                </tbody>
            </table>
        </section>

        <footer>
            <p>Generated `)
	buf.WriteString(clocky.Now().String()[:19])
	buf.WriteString(` &middot; Data from Coinbase</p>
        </footer>
    </div>
    <script src="https://justinestreet.capital/static/js/lightweight-charts-5.1.0.js"></script>
    <script src="chart.js"></script>
</body>
</html>
`)

	return buf.String()
}

// formatDuration formats a duration for display.
func formatDuration(d clocky.Duration) string {
	days := int(d / clocky.Day)
	hours := int((d % clocky.Day) / clocky.Hour)
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	return fmt.Sprintf("%dh", hours)
}

// writeFileAtomic writes a file atomically by writing to a temp file and renaming.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
