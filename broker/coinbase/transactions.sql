-- Coinbase V2 Transactions Schema
--
-- This table stores transactions from the Coinbase V2 API (/v2/accounts/{id}/transactions).
-- It is used for accounting, P/L tracking, and reconciliation with live balances.
--
-- CRITICAL FINDINGS FROM RECONCILIATION ANALYSIS (2025-12-31):
--
-- 1. NATIVE_AMOUNT ACCURACY
--    The native_amount field accurately represents the USD notional (price × quantity)
--    being credited or debited. It is ALWAYS rounded to whole cents (2 decimal places).
--    Fees are NOT included in native_amount - they are charged separately.
--
-- 2. FILL_PRICE IS NOT THE EXECUTION PRICE
--    The fill_price field appears to be a reference or limit price, NOT the actual
--    execution price. The true execution price is: native_amount / amount.
--    In our analysis, the implied price was MORE favorable than fill_price in ~64%
--    of fills, suggesting price improvement is common.
--
-- 3. FEES ARE SEPARATE
--    Trading fees are recorded in fill_commission and charged separately from
--    native_amount. Total USD cost for a buy = ABS(native_amount) + fill_commission.
--    Total USD received for a sell = ABS(native_amount) - fill_commission.
--
-- 4. DUAL TRANSACTION RECORDS FOR FILLS
--    Each advanced_trade_fill creates TWO transaction records:
--      - One in the crypto account (e.g., BTC) with the crypto amount
--      - One in the USD account with the USD amount
--    Both share the same fill_order_id but have different account_id values.
--    The crypto record has fill_commission; the USD record does not.
--
-- 5. RECONCILIATION INVARIANTS
--    For crypto accounts:
--      SUM(amount) WHERE currency='BTC' = live BTC balance (EXACT MATCH expected)
--    For USD accounts:
--      SUM(amount) WHERE currency='USD' - SUM(fill_commission) ≈ live USD balance
--      Small discrepancies (~$1 per $50K volume) are normal due to penny rounding.
--
-- 6. ROUNDING BEHAVIOR
--    Coinbase rounds native_amount to the nearest cent (not systematically against
--    the user). Analysis showed rounding slightly favored the user (~$0.58 over
--    1600 fills). This is fair rounding, not "Office Space" style theft.
--
-- 7. PORTFOLIO ISOLATION
--    Each portfolio (dropbear, zec, primary) has its own accounts with unique UUIDs.
--    The V2 API returns ALL accounts across portfolios, so when syncing, you MUST
--    filter by portfolio_id to avoid cross-contamination. The V3 API (/api/v3/...)
--    returns only portfolio-specific accounts based on the API key used.
--
-- 8. TRANSACTION TYPES
--    - advanced_trade_fill: Fills from Advanced Trade orders
--    - tx: Transfers between portfolios or deposits/withdrawals
--    - send/receive: On-chain crypto transfers
--    - buy/sell: Legacy simple buy/sell (fiat gateway)
--    - trade: Crypto-to-crypto conversions
--
-- 9. SIGN CONVENTIONS
--    - amount: Positive = credit to account, Negative = debit from account
--    - For buys: crypto amount is positive, USD amount is negative
--    - For sells: crypto amount is negative, USD amount is positive
--    - native_amount follows the same sign as amount
--

CREATE TABLE IF NOT EXISTS coinbase_transactions (

	-- IDENTITY
	id TEXT NOT NULL PRIMARY KEY,   -- Unique transaction ID (UUID format)
	account_id TEXT NOT NULL,       -- V2 account UUID (portfolio-specific, one per currency per portfolio)

	-- METADATA
	type TEXT NOT NULL,             -- Transaction type: advanced_trade_fill, tx, send, receive, buy, sell, trade
	status TEXT NOT NULL,           -- Status: pending, completed, failed, cancelled
	created_at INTEGER NOT NULL,    -- Unix microseconds (API gives seconds, we store micros for consistency)
	updated_at INTEGER,             -- Unix microseconds, NULL if never updated

	-- AMOUNTS
	-- These are the authoritative values for accounting. SUM(amount) = live balance for crypto.
	amount TEXT NOT NULL,           -- Quantity in native units (e.g., "0.12345678" BTC)
	                                -- Positive = credit, Negative = debit
	                                -- Precision: up to 8 decimal places for crypto, 2 for fiat
	currency TEXT NOT NULL,         -- Currency code: BTC, ETH, USD, USDC, etc.
	native_amount TEXT NOT NULL,    -- USD equivalent at time of transaction, rounded to cents
	                                -- This is the ACTUAL notional (price × qty), not a reference value
	                                -- Does NOT include fees - those are in fill_commission
	native_currency TEXT NOT NULL,  -- Always "USD" for US accounts

	-- FILL DETAILS (for type = 'advanced_trade_fill')
	-- Note: fill_price is NOT the execution price! Use native_amount/amount for true price.
	fill_order_id TEXT,             -- Order UUID that generated this fill
	fill_product_id TEXT,           -- Trading pair: BTC-USD, ETH-USD, etc.
	fill_side TEXT,                 -- "buy" or "sell" (from order perspective)
	fill_price TEXT,                -- Reference/limit price - NOT actual execution price!
	                                -- True execution price = ABS(native_amount) / ABS(amount)
	                                -- fill_price is often WORSE than actual execution (price improvement)
	fill_commission TEXT,           -- Fee in quote currency (USD), charged SEPARATELY from native_amount
	                                -- Only present on crypto-side records, not USD-side records

	-- IDEMPOTENCY
	idem TEXT,                      -- Idempotency key for send transactions

	-- ON-CHAIN DETAILS (for type = 'send' or 'receive')
	network_hash TEXT,              -- Blockchain transaction hash
	network_name TEXT,              -- Network: bitcoin, ethereum, litecoin, etc.
	network_fee TEXT,               -- On-chain fee paid (in native crypto units)

	-- RECIPIENT DETAILS (for type = 'send')
	to_resource TEXT,               -- Resource type: address, user, account
	to_address TEXT,                -- Blockchain address (if external)
	to_id TEXT,                     -- Coinbase user/account ID (if internal)
	to_email TEXT,                  -- Recipient email (if Coinbase user)
	to_name TEXT,                   -- Recipient name

	-- SENDER DETAILS (for type = 'receive')
	from_resource TEXT,             -- Resource type: address, user, account
	from_address TEXT,              -- Blockchain address (if external)
	from_id TEXT,                   -- Coinbase user/account ID (if internal)
	from_email TEXT,                -- Sender email (if Coinbase user)
	from_name TEXT,                 -- Sender name

	-- LEGACY BUY DETAILS (for type = 'buy' via fiat gateway)
	buy_id TEXT,
	buy_fee TEXT,                   -- Fee charged for the buy
	buy_total TEXT,                 -- Total fiat charged (subtotal + fee)
	buy_subtotal TEXT,              -- Fiat amount before fee
	buy_payment_method_name TEXT,   -- Payment method used (bank, card, etc.)

	-- LEGACY SELL DETAILS (for type = 'sell' via fiat gateway)
	sell_id TEXT,
	sell_fee TEXT,                  -- Fee charged for the sell
	sell_total TEXT,                -- Total fiat received (subtotal - fee)
	sell_subtotal TEXT,             -- Fiat amount before fee
	sell_payment_method_name TEXT   -- Payout method (bank, PayPal, etc.)
);

-- INDEXES

-- Fast lookup by currency (e.g., get all BTC transactions)
CREATE INDEX IF NOT EXISTS idx_coinbase_transactions_currency ON coinbase_transactions(currency);

-- Fast time-range queries (e.g., transactions in last 24h)
CREATE INDEX IF NOT EXISTS idx_coinbase_transactions_created_at ON coinbase_transactions(created_at);

-- Fast lookup by type (e.g., all fills vs all transfers)
CREATE INDEX IF NOT EXISTS idx_coinbase_transactions_type ON coinbase_transactions(type);

-- Fast lookup by account (used by SyncTransactions for incremental sync)
CREATE INDEX IF NOT EXISTS idx_coinbase_transactions_account_id ON coinbase_transactions(account_id);

-- Partial index for efficiently finding unconfirmed on-chain transactions.
-- Used by SyncTransactions to re-sync transactions that may have updated
-- (e.g., gaining confirmations, changing from pending to completed).
CREATE INDEX IF NOT EXISTS idx_coinbase_transactions_unconfirmed_network
ON coinbase_transactions(account_id, created_at)
WHERE network_hash IS NOT NULL AND network_hash != '' AND status != 'completed';

-- EXAMPLE QUERIES

-- Calculate live crypto balance (should EXACTLY match Coinbase):
--   SELECT SUM(CAST(amount AS REAL)) FROM coinbase_transactions
--   WHERE currency = 'BTC' AND status = 'completed';

-- Calculate total fees paid:
--   SELECT SUM(CAST(fill_commission AS REAL)) FROM coinbase_transactions
--   WHERE fill_commission != '';

-- Get true execution price for a fill:
--   SELECT ABS(CAST(native_amount AS REAL)) / ABS(CAST(amount AS REAL)) as exec_price
--   FROM coinbase_transactions WHERE type = 'advanced_trade_fill';

-- Reconcile USD balance (deposits - notionals - fees ≈ live balance):
--   SELECT
--     SUM(CASE WHEN type = 'tx' THEN CAST(amount AS REAL) ELSE 0 END) as deposits,
--     SUM(CASE WHEN type = 'advanced_trade_fill' THEN CAST(amount AS REAL) ELSE 0 END) as fill_flows,
--     (SELECT SUM(CAST(fill_commission AS REAL)) FROM coinbase_transactions WHERE fill_commission != '') as fees
--   FROM coinbase_transactions WHERE currency = 'USD';
