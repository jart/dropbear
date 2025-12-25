CREATE TABLE IF NOT EXISTS coinbase_minutecandles (
	symbol TEXT NOT NULL,
	start INTEGER NOT NULL,
	open TEXT NOT NULL,
	high TEXT NOT NULL,
	low TEXT NOT NULL,
	close TEXT NOT NULL,
	volume TEXT NOT NULL,
	PRIMARY KEY (symbol, start)
);

CREATE INDEX IF NOT EXISTS idx_minutecandles_symbol ON coinbase_minutecandles(symbol);
CREATE INDEX IF NOT EXISTS idx_minutecandles_symbol_start ON coinbase_minutecandles(symbol, start);
