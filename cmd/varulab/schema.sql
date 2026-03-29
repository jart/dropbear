CREATE TABLE IF NOT EXISTS varulab_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol TEXT NOT NULL,
    date TEXT NOT NULL,
    flags TEXT NOT NULL,
    dbn_path TEXT NOT NULL,
    git_rev TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    winning INTEGER,
    balance INTEGER,
    fees INTEGER,
    log TEXT,
    started_at INTEGER,
    finished_at INTEGER,
    created_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_varulab_runs_dedup
    ON varulab_runs(symbol, date, flags, git_rev);
CREATE INDEX IF NOT EXISTS idx_varulab_runs_status
    ON varulab_runs(status);
CREATE INDEX IF NOT EXISTS idx_varulab_runs_symbol_date
    ON varulab_runs(symbol, date);
CREATE INDEX IF NOT EXISTS idx_varulab_runs_dbn_path
    ON varulab_runs(dbn_path);

CREATE TABLE IF NOT EXISTS varulab_flag_sets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    flag TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_varulab_flag_sets_dedup
    ON varulab_flag_sets(flag, value);
