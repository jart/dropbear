# dropbear coding guidelines

this project does equities trading using go.

## commands

- `go fmt ./...`
- `go test ./...`
- never use `go build` (it litters executables) just write a test instead even if it does nothing

## testing

- please write benchmarks too if it makes sense
- use deterministic random seeds for reproducibility
- we'd like to be more formal and safe than we are
- try to tease out edge cases and corner cases
- call `ds.SetOffline()` in `init()` so you don't accidentally live trade

## packages

- `cubby/` is our QuantConnect-like framework for writing equity trading algorithms
- `teddy/` is our QuantConnect-like framework for writing crypto trading algorithms
- `clocky/` is our time library
- `loggy/` is our logging utilities
- `decimal/` our fixed point number library with 8 decimal places
- `db/` use `db.Get()` to get a WAL2 SQLite singleton into `~/.dropbear.sqlite3`
- `indicators/` has indicators similar quantconnect but better, defines candles
- `broker/alpaca/` is our client library for alpaca brokerage
- `broker/databento/` is our client library for getting order book data
- `auth/` lets the dropbear https server support yubikey authentication

## equity bars

Files like `~/equitydata/minutes/AAPL` can be read using `ds.Bars` which mmaps them into
memory one great big array that can be seeked, indexed, etc. They contain OHLC / VWAP data.
This is *the* data that drives all our equities trading backtests and algorithms. This code
is the reason why dropbear goes infinitely faster than quantconnect.

If you need data for a stock that hasn't been downloaded yet, all you have to do is run a
command like `go run ./broker/alpaca/cmd/download GOOG SPY QQQ TSLA PLTR GOOGL`. If the bars
have already been downloaded, then this command will sync the latest data.

If you want to do analysis on equity bars, then sqlite is easier to use than the binary format.
You can run `go run ./broker/alpaca/cmd/sqlifybars SPY` to copy the bars to `~/.dropbear.sqlite3`.

## style

- we want algorithms to have optimal time complexity. you're gonna have to rewrite it if it isn't
- we try to avoid dependencies (ask before introducing)
- we almost never use IEEE floating point for financial code
- quantize decimals later when you must (never do it early just cuz)
- we like vendoring static web assets using go:embed
- never embed css/js in html (create separate files)

## decimal library

Use our decimal library for everything. Please don't ever use floating point math. It
stores six decimal places in a single `int64` word, just like CTS.

- `decimal.Parse("0.01")`
- `decimal.FromInt(100)`
- `bid.Add(ask).DivInt(2)` calculates midpoint
- `x.Cmp(y)` for comparisons
- `x.{Min,Max}(y)` is nice and terse
- `d.String()` produces string that shows decimal places be removes trailing zeroes
- `d.Format(2)` produces string that always has 2 decimal places with nearest rounding
- `d.MulInt(2)` shortcut for `d.Mul(decimal.Parse("2"))`
- `d.Int()` and `d.Int64()` exist, also Sqr, Exp, Neg, Abs, IsZero, IsPositive, etc.
- `d.QuantizeEven(q)` rounds half to even
- `d.QuantizeNearest(q)` rounds half away from zero
- `d.QuantizeTruncate(q)` rounds towards zero (use for order sizing and bid prices)
- `d.QuantizeAway(q)` rounds away from zero (use for margin calculations and ask prices)

## time and durations

Please use our `clocky` library instead of Go's `time` library for everything. It stores
unix nanoseconds in a single `int64` word.

- `clocky.Time`
- `clocky.Duration`
- Use `clocky.Now()` becasue it can be mocked for backtesting.

## running backtests

our best equities trading strategy is

```bash
go run ./cmd/holder -backtest -start 2025-10-01 -symbol "GOOG"
```

which says

```
2026-01-16T17:00:00.000000000 backtest completed: 51972 iterations
2026-01-16T17:00:00.000000000 summary:
2026-01-16T17:00:00.000000000   start:    $100,000.00
2026-01-16T17:00:00.000000000   end:      $183,128.79
2026-01-16T17:00:00.000000000   fees:     $10.80
2026-01-16T17:00:00.000000000   interest: $2,483.15
2026-01-16T17:00:00.000000000   max dd:   19.43%
2026-01-16T17:00:00.000000000   return:   83.13% (281.79% annualized)
2026-01-16T17:00:00.000000000   bench:    37.05% (125.61% annualized) [GOOG]
2026-01-16T17:00:00.000000000   period:   107.8 days (0.30 years)
2026-01-16T17:00:00.000000000 holdings:
2026-01-16T17:00:00.000000000      USD $-175,748.01 margin $107,663.04
2026-01-16T17:00:00.000000000     GOOG     1088 shares @ $329.85 = $358,876.80
```
