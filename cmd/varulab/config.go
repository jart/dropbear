package main

import "os"

// program is the program to run in each backtest if -program is not provided.
const kProgram = "./cmd/scalp"

// defaultSymbols is the default set of symbols to backtest if -symbols is not provided.
const kDefaultSymbols = "NVDA TSLA GOOGL META AAPL MSFT AMZN"

// earliestDate is the oldest date the downloader will fetch.
const kEarliestDate = "2025-05-01"

// dataDirs is the list of databento directories in order of preference.
// Downloads go to the first one that exists and has <90% disk usage.
// These directories are not created automatically.
var kDataDirs = []string{
	"/fast/databento",
	"/disk/databento",
	os.ExpandEnv("$HOME/databento"),
}

// kBaseFlags are included in every backtest run.
var kBaseFlags = ""

// kFlagDimensions defines the search space. Each inner slice is a dimension;
// the Cartesian product of all dimensions (plus a baseline with each dimension
// absent) generates the full set of flag combinations to test.
var kFlagDimensions = [][]string{
	{"-direction=1"},
}
