CREATE TABLE IF NOT EXISTS coinbase_transactions (
	id TEXT NOT NULL PRIMARY KEY, -- transaction id
	account_id TEXT NOT NULL, -- account id
	type TEXT NOT NULL, -- send, buy, sell, advanced_trade_fill, etc.
	status TEXT NOT NULL, -- pending, completed, failed, etc.
	created_at INTEGER NOT NULL, -- unix microsecond timestamp (api only gives second precision)
	updated_at INTEGER, -- unix microsecond timestamp (api only gives second precision)
	amount TEXT NOT NULL,
	currency TEXT NOT NULL,
	native_amount TEXT NOT NULL,
	native_currency TEXT NOT NULL,
	fill_order_id TEXT,
	fill_product_id TEXT,
	fill_side TEXT,
	fill_price TEXT,
	fill_commission TEXT,
	idem TEXT, -- idempotency key (currently only used for send transactions)
	network_hash TEXT,
	network_name TEXT,
	network_fee TEXT,
	to_resource TEXT,
	to_address TEXT,
	to_id TEXT,
	to_email TEXT,
	to_name TEXT,
	from_resource TEXT,
	from_address TEXT,
	from_id TEXT,
	from_email TEXT,
	from_name TEXT,
	buy_id TEXT,
	buy_fee TEXT,
	buy_total TEXT,
	buy_subtotal TEXT,
	buy_payment_method_name TEXT,
	sell_id TEXT,
	sell_fee TEXT,
	sell_total TEXT,
	sell_subtotal TEXT,
	sell_payment_method_name TEXT
);

CREATE INDEX IF NOT EXISTS idx_coinbase_transactions_currency ON coinbase_transactions(currency);
CREATE INDEX IF NOT EXISTS idx_coinbase_transactions_created_at ON coinbase_transactions(created_at);
CREATE INDEX IF NOT EXISTS idx_coinbase_transactions_type ON coinbase_transactions(type);
CREATE INDEX IF NOT EXISTS idx_coinbase_transactions_account_id ON coinbase_transactions(account_id);

-- Partial index for efficiently finding unconfirmed on-chain transactions
-- Used by SyncTransactions to re-sync transactions that may have updated
CREATE INDEX IF NOT EXISTS idx_coinbase_transactions_unconfirmed_network
ON coinbase_transactions(account_id, created_at)
WHERE network_hash IS NOT NULL AND network_hash != '' AND status != 'completed';
