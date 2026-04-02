package main

import "os"

// defaultSymbols is the default set of symbols to backtest if -symbols is not provided.
// const defaultSymbols = "MSFT AVGO TSLA NVDA META GOOGL XSP" // META XSP SPXW RUTW
// const defaultSymbols = "MSFT META AVGO NVDA TSLA GOOGL AAPL"
const defaultSymbols = "SPXW RUTW XSP"

// earliestDate is the oldest date the downloader will fetch.
const earliestDate = "2026-01-01"

// dataDirs is the list of databento directories in order of preference.
// Downloads go to the first one that exists and has <90% disk usage.
// These directories are not created automatically.
var dataDirs = []string{
	"/fast/databento",
	"/disk/databento",
	os.ExpandEnv("$HOME/databento"),
}

// kBaseFlags are included in every backtest run.
// var kBaseFlags = "-bearish -eval=3 -hostile -spread=.5 -w-delta=0 -w-risk=0 -sod=100000"
var kBaseFlags = "-hostile -spread=.5"

// kFlagDimensions defines the search space. Each inner slice is a dimension;
// the Cartesian product of all dimensions (plus a baseline with each dimension
// absent) generates the full set of flag combinations to test.
var kFlagDimensions = [][]string{
	// {"-bullish -w-delta=0", "-bearish -w-delta=0"},
	// {"-eval=3", "-eval=5", "-eval=7", "-eval=10"}, // this makes varu pickier about trades
	// {"-spread=.5"}, // this is usually needed when liquidity is tight (since varu crosses the spread by default)
	// {"-w-risk=.5", "-w-delta=.5", "-w-payoff=.5"},
	// {"-budget=2000"},
	// {"-sod=100000"},
	{"-eval=10", "-eval=7", "-eval=5"},
	{"-sigmas=3"},
}

// 2026-03-29 (find good stocks)
// NVDA: $7221/day w/ -bearish -hostile -spread=1 -w-delta=0 -w-risk=0 (26 tests)
// TSLA: $4908/day w/ -bearish -eval=3 -hostile -sigmas=1.5 -spread=.5 -w-delta=0 -w-risk=0 (27 tests)
// META: $3501/day w/ -hostile -sigmas=2 (27 tests)
// NFLX: $3367/day w/ -bullish -hostile -sigmas=1.5 -spread=1 -w-delta=0 -w-risk=0 (12 tests)
// AMZN: $2510/day w/ -bullish -eval=4 -hostile -sigmas=2 -spread=1 -w-delta=0 -w-risk=0 (12 tests)
// SPXW: $2449/day w/ -bearish -eodt=140000 -eval=3 -frt=110000 -hostile -sod=94500 -spread=.5 -w-delta=0 -w-risk=0 (58 tests)
// XSP:  $1891/day w/ -eval=4 -hostile -sigmas=2 -spread=1 (58 tests)
// COIN: $1798/day w/ -bullish -hostile -sigmas=2 -spread=1 -w-delta=0 -w-risk=0 (12 tests)
// ORCL: $1700/day w/ -bearish -hostile -sigmas=1.5 -spread=.5 -w-delta=0 -w-risk=0 (12 tests)
// IWM:  $1291/day w/ -bullish -eval=3 -hostile -spread=1 -w-delta=0 -w-risk=0 (53 tests)
// INTC: $898/day w/ -bearish -hostile -spread=1 -w-delta=0 -w-risk=0 (12 tests)
// ADBE: $650/day w/ -bearish -hostile -sigmas=2 -spread=1 -w-delta=0 -w-risk=0 (12 tests)
// SHOP: $376/day w/ -bearish -eval=3 -hostile -spread=1 -w-delta=0 -w-risk=0 (12 tests)
// BAC:  $126/day w/ -bullish -hostile -spread=.5 -w-delta=0 -w-risk=0 (12 tests)

// 2026-03-29 (find good time flags)
// winner: -sod=100000 helps a bit
// {"-sod=94500", "-sod=100000"},
// {"-frt=110000", "-frt=130000"},
// {"-eodt=120000", "-eodt=140000"},

// 2026-03-29 (find optimal -w-foo flags)
// winner: -w-delta=0 -w-risk=0
// {"-w-delta=0", "-w-delta=3", "-w-delta=10"},
// {"-w-risk=0", "-w-risk=3", "-w-risk=10"},
// {"-w-payoff=.5", "-w-payoff=3"},

// 2026-03-28 (my first grid search)
// winner: -bearish -eval=3 -hostile -spread=.5
// {"-eval=3", "-eval=4"},
// {"-spread=.5", "-spread=1"},
// {"-floor=20_000 -budget=2_500"},
// {"-sigmas=2"},
