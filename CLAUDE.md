# dropbear coding guidelines

this project does options and equities trading using go.

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

- `cmd/varu/` our flagship options trading program
- `clocky/` is our time library
- `loggy/` is our logging utilities
- `decimal/` our fixed point number library with 8 decimal places
- `db/` use `db.Get()` to get a WAL2 SQLite singleton into `~/.dropbear.sqlite3`
- `indicators/` has indicators similar quantconnect but better, defines candles
- `broker/schwab/` is our client library for schwab brokerage
- `broker/alpaca/` is our client library for alpaca brokerage
- `broker/databento/` is our client library for getting order book data
- `cubby/` is our QuantConnect-like framework for writing equity trading algorithms
- `auth/` lets the dropbear https server support yubikey authentication

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
- `bid.Add(ask).DivInt(2)` or `bid.Add(ask).Half()` calculates midpoint
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

