package coinbase

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// GetLots returns remaining inventory lots after replaying all transactions.
// Buys/receives add lots, sells/sends consume lots using the specified method.
// You need to call SyncTransactions(currency) beforehand.
func (c *Client) GetLots(currency string, method ds.CostBasisMethod) (*ds.Lots, error) {
	rows, err := c.db.Query(`
		SELECT created_at, type, amount, native_amount, to_address,
		       buy_total, fill_side, fill_price, fill_commission, fill_product_id
		FROM coinbase_transactions
		WHERE currency = ? AND status = 'completed'
		ORDER BY created_at ASC
	`, currency)
	if err != nil {
		return nil, fmt.Errorf("querying transactions: %w", err)
	}
	defer rows.Close()
	lots := ds.NewLots(method)
	for rows.Next() {
		var createdAtMicros int64
		var txType, amountStr, nativeAmountStr string
		var toAddress, buyTotal, fillSide, fillPrice, fillCommission, fillProductID *string
		if err := rows.Scan(&createdAtMicros, &txType, &amountStr, &nativeAmountStr, &toAddress,
			&buyTotal, &fillSide, &fillPrice, &fillCommission, &fillProductID); err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}
		createdAt := clocky.Time(createdAtMicros)
		amount := decimal.Parse(amountStr)
		absAmount := amount.Abs()
		switch txType {
		case "buy":
			cost := decimal.Parse(*buyTotal)
			lots.Add(createdAt, absAmount, cost)
		case "send", "tx":
			if amount.IsPositive() {
				cost := decimal.Parse(nativeAmountStr).Abs()
				lots.Add(createdAt, absAmount, cost)
			} else {
				lots.Consume(absAmount, decimal.Zero)
			}
		case "advanced_trade_fill":
			if amount.IsPositive() {
				var cost decimal.Decimal
				if fillProductID != nil && strings.HasSuffix(*fillProductID, "-USD") {
					price := decimal.Parse(*fillPrice)
					commission := decimal.Parse(*fillCommission)
					cost = absAmount.Mul(price).Add(commission)
				} else {
					cost = decimal.Parse(nativeAmountStr).Abs()
				}
				lots.Add(createdAt, absAmount, cost)
			} else {
				lots.Consume(absAmount, decimal.Zero)
			}
		case "sell", "credit_card_balance_payment":
			lots.Consume(absAmount, decimal.Zero)
		case "interest", "subscription_rebate", "credit_card_reward":
			cost := decimal.Parse(nativeAmountStr).Abs()
			lots.Add(createdAt, absAmount, cost)
		default:
			return nil, fmt.Errorf("unknown transaction type %q at %s", txType, createdAt)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}
	return lots, nil
}
