package main

import (
	"bytes"
	"database/sql"
	"dropbear/broker/coinbase"
	"dropbear/clocky"
	"dropbear/db"
	"dropbear/decimal"
	"dropbear/ds"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"strings"
)

//go:embed static
var staticFiles embed.FS

// ChartData is the JSON structure for the chart.
type ChartData struct {
	Strategy  []ChartPoint `json:"strategy"`
	Benchmark []ChartPoint `json:"benchmark"`
	Metrics   struct {
		TotalReturn     float64 `json:"totalReturn"`
		CAGR            float64 `json:"cagr"`
		Sharpe          float64 `json:"sharpe"`
		MaxDrawdown     float64 `json:"maxDrawdown"`
		BenchmarkReturn float64 `json:"benchmarkReturn"`
	} `json:"metrics"`
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
	snapshots, metrics, err := calculateMetrics(client, asset, genesis, quantum)
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

	// build chart data JSON
	chartData := buildChartData(snapshots, metrics)
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
	account, err := client.GetV2AccountByCurrencyCode(asset)
	if err != nil {
		log.Printf("warning: failed to get v2 account for %s: %v", asset, err)
		return "unavailable", ""
	}

	addresses, err := client.GetDepositAddresses(account.ID)
	if err != nil {
		log.Printf("warning: failed to get deposit addresses: %v", err)
		return "unavailable", ""
	}

	if len(addresses) == 0 {
		// try to create one
		addr, err := client.CreateDepositAddress(account.ID)
		if err != nil {
			log.Printf("warning: failed to create deposit address: %v", err)
			return "unavailable", ""
		}
		return addr.Address, addr.Network
	}

	return addresses[0].Address, addresses[0].Network
}

// fetchTransactionsForDisplay fetches transactions for the HTML table.
// Includes LIFO profit/loss calculation for sell transactions.
func fetchTransactionsForDisplay(client *coinbase.Client, asset string, genesis clocky.Time) ([]TransactionRow, error) {
	// get LIFO lots for profit/loss calculation
	lots, err := client.GetLots(asset, ds.CostBasisMethodLIFO)
	if err != nil {
		log.Printf("warning: failed to get lots for profit/loss: %v", err)
	}

	database := db.Get()
	rows, err := database.Query(`
		SELECT type, status, created_at, amount, native_amount,
		       COALESCE(fill_side, ''), COALESCE(fill_price, ''), COALESCE(fill_commission, '')
		FROM coinbase_transactions
		WHERE currency = :currency
		  AND created_at >= :genesis
		ORDER BY created_at DESC
		LIMIT 1000
	`, sql.Named("currency", asset), sql.Named("genesis", int64(genesis)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TransactionRow
	for rows.Next() {
		var txType, status, amountStr, nativeStr, fillSide, fillPriceStr, commissionStr string
		var createdAt int64
		if err := rows.Scan(&txType, &status, &createdAt, &amountStr, &nativeStr, &fillSide, &fillPriceStr, &commissionStr); err != nil {
			continue
		}

		t := clocky.Time(createdAt)
		amount := decimal.Parse(amountStr)
		native := decimal.Parse(nativeStr)
		commission := decimal.Parse(commissionStr)
		fillPrice := decimal.Parse(fillPriceStr)

		row := TransactionRow{
			Date:     t.String()[:19], // trim microseconds
			Type:     formatTxType(txType),
			Amount:   formatAmount(amount, asset, 8),
			Notional: formatAmount(native, "USD", 2),
			Status:   status,
		}

		// show side if present
		if fillSide == "buy" {
			row.Side = "Buy"
		} else if fillSide == "sell" {
			row.Side = "Sell"
		}

		// show fill price if present
		if !fillPrice.IsZero() {
			row.FillPrice = formatAmount(fillPrice, "USD", 2)
		}

		// show fee if present
		if !commission.IsZero() {
			row.Fee = formatAmount(commission.Neg(), "USD", 8) // show as negative (cost)
		}

		// calculate LIFO cost basis and profit/loss for sells
		if fillSide == "sell" && lots != nil && !fillPrice.IsZero() {
			sellQty := amount.Neg() // amount is negative for sells
			proceeds := sellQty.Mul(fillPrice)
			costBasis := lots.GetCostBasis(sellQty, fillPrice)
			profitLoss := proceeds.Sub(costBasis).Sub(commission)
			row.CostBasis = formatAmount(costBasis, "USD", 8)
			row.ProfitLoss = formatAmount(profitLoss, "USD", 8)
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

// buildChartData creates the chart data structure.
func buildChartData(snapshots []PortfolioSnapshot, m *ReportMetrics) ChartData {
	data := ChartData{}
	data.Metrics.TotalReturn = m.TotalReturn
	data.Metrics.CAGR = m.CAGR
	data.Metrics.Sharpe = m.Sharpe
	data.Metrics.MaxDrawdown = m.MaxDrawdown
	data.Metrics.BenchmarkReturn = m.BenchmarkReturn

	for _, s := range snapshots {
		// use Unix seconds for lightweight charts
		unixSec := s.Time.Unix()
		data.Strategy = append(data.Strategy, ChartPoint{
			Time:  unixSec,
			Value: s.StrategyValue.Float64(),
		})
		data.Benchmark = append(data.Benchmark, ChartPoint{
			Time:  unixSec,
			Value: s.BenchmarkValue.Float64(),
		})
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
    <title>Coinbase `)
	buf.WriteString(html.EscapeString(asset))
	buf.WriteString(`-USD Spot Trading</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <div class="container">
        <h1>COINBASE `)
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
            <div class="metric">
                <span class="label">Total Return</span>
                <span class="value`)
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
                <span class="label">Annualized</span>
                <span class="value`)
	if m.CAGR >= 0 {
		buf.WriteString(" positive")
	} else {
		buf.WriteString(" negative")
	}
	buf.WriteString(`">`)
	buf.WriteString(decimal.FromFloat64(m.CAGR).FormatThousand(1) + "%")
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label">Sharpe Ratio</span>
                <span class="value">`)
	buf.WriteString(fmt.Sprintf("%.2f", m.Sharpe))
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label">Max Drawdown</span>
                <span class="value negative">`)
	buf.WriteString(fmt.Sprintf("-%.1f%%", m.MaxDrawdown))
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label">vs Benchmark</span>
                <span class="value`)
	if alpha >= 0 {
		buf.WriteString(" positive")
	} else {
		buf.WriteString(" negative")
	}
	buf.WriteString(`">`)
	buf.WriteString(fmt.Sprintf("%+.1f%%", alpha))
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
	buf.WriteString(m.TotalRealized.Abs().FormatThousand(2))
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
	buf.WriteString(m.TotalUnrealized.Abs().FormatThousand(2))
	buf.WriteString(`</span>
            </div>
            <div class="metric">
                <span class="label">Fees Paid</span>
                <span class="value negative">-$`)
	buf.WriteString(m.FeesPaid.FormatThousand(2))
	buf.WriteString(`</span>
            </div>
        </section>

        <section class="chart-section">
            <div id="chart"></div>
        </section>

        <section class="portfolio">
            <h2>PORTFOLIO VALUE</h2>
            <div class="portfolio-value">$`)
	buf.WriteString(m.CurrentValue.FormatThousand(2))
	buf.WriteString(`</div>
            <div class="holdings">
                <span class="holding">`)
	buf.WriteString(m.AssetBalance.String())
	buf.WriteString(" ")
	buf.WriteString(html.EscapeString(asset))
	buf.WriteString(` @ $`)
	buf.WriteString(m.AssetPrice.FormatThousand(2))
	buf.WriteString(`</span>
                <span class="holding">$`)
	buf.WriteString(m.USDBalance.FormatThousand(2))
	buf.WriteString(` USD</span>
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
	buf.WriteString(` to this address, you get the benefit of entertainment in seeing the crypto show up on this page and being traded, and the satisfaction of knowing that you're supporting her work in fields such as open source development, artificial intelligence, finance, and grassroots activism. <strong>This is not an investment.</strong> You are giving her the crypto. She has full autonomy over how she uses her crypto but promises you'll at least get to see it in action as part of this live trading portfolio for a short time. She has no way of knowing who sent her the crypto unless you tell her. You should furthermore consider the tax implications of sharing cryptography.
            </p>
            <p class="disclaimer">
			    Our bespoke algorithm works by performing international surveillance of publicly available information in the cryptography community and then routing that knowledge to New York over private networks for analysis by proprietary Go code that places marketable IOC limit orders via the <a href="https://advanced.coinbase.com/join/L8EN839">Coinbase Advanced Trading API</a> as a VIP 4 member in order to clean stale bids and asks from the order book. You should assume that this algorithm is highly risky and experimental. It may lose all value at any time, including during periods of apparent stability. Past performance is no guarantee of future results. If you use our Coinbase referral hyperlink to join the website, you should not expect to see similar returns. They will charge you much higher fees and trying to make money off cryptography is like trying to milk a male tiger. Don't come into the den of jackals. Do anything else instead, like buy U.S. Treasury Bonds from Charles Schwab.
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
		feeClass := "num"
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
