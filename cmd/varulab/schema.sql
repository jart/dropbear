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

-- dedup: prevent re-running same experiment
CREATE UNIQUE INDEX IF NOT EXISTS idx_varulab_runs_dedup2
    ON varulab_runs(symbol, date, flags);

-- scheduler: find pending runs, prefer by dbn_path
CREATE INDEX IF NOT EXISTS idx_varulab_runs_pending
    ON varulab_runs(status, dbn_path) WHERE status = 'pending';

-- status page: running jobs sorted by start time
CREATE INDEX IF NOT EXISTS idx_varulab_runs_running
    ON varulab_runs(status, started_at DESC) WHERE status = 'running';

-- status page: recent completed/failed sorted by finish time
CREATE INDEX IF NOT EXISTS idx_varulab_runs_finished
    ON varulab_runs(status, finished_at DESC);

-- summary/grid: covering index for aggregate queries
CREATE INDEX IF NOT EXISTS idx_varulab_runs_summary
    ON varulab_runs(status, date, symbol, flags, winning, fees);

-- grid: best winning per symbol/date
CREATE INDEX IF NOT EXISTS idx_varulab_runs_grid
    ON varulab_runs(status, symbol, date, winning DESC);

-- counts: status distribution
CREATE INDEX IF NOT EXISTS idx_varulab_runs_status
    ON varulab_runs(status);

-- cleanup: find pending runs by symbol
CREATE INDEX IF NOT EXISTS idx_varulab_runs_symbol_status
    ON varulab_runs(symbol, status);

CREATE TABLE IF NOT EXISTS varulab_flag_sets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    flag TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_varulab_flag_sets_dedup
    ON varulab_flag_sets(flag, value);
