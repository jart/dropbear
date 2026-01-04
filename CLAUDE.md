# dropbear coding guidelines

this project does equities and cryptography trading using go.

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
- `cmd/spread/` is an example of a trading bot that uses the teddy framework
- `loggy/` is our logging utilities
- `decimal/` our fixed point number library with 8 decimal places
- `db/` use `db.Get()` to get a WAL2 SQLite singleton into `~/.dropbear.sqlite3`
- `indicators/` has indicators similar quantconnect but better, defines candles
- `orderbook/` uses gods v2 tree set for fast level2 order book management
- `broker/coinbase/` our own client library (where we currently do our trading!)
- `broker/binance/` our own client library (used for market data only currently)
- `broker/alpaca/` our own client library (that's where we trade equities)
- `auth/` lets the dropbear https server support yubikey authentication

## data locations

- `~/.dropbear.sqlite3` - main database for fills/trades. has 20gb of equity candles too
- `~/equitydata/minutes/AAPL/2025-01` - indicators.Candle binary market data
- `~/marketdata/<dataset>/{coinbase,binance}/<SYMBOL>` - ds.Tick binary market data

## programs

- `cmd/spread/` is the main trading strategy (live and backtest)
- `cmd/dumptick ~/marketdata/weekend/binance/FDUSDUSDT` dumps raw recorded data
- `cmd/record.{binance,coinbase}/` records live websocket data to binary format

## style

- we want algorithms to have optimal time complexity. you're gonna have to rewrite it if it isn't
- we try to avoid dependencies (ask before introducing)
- we almost never use IEEE floating point for financial code
- it's ok to sanity check and runtime panic with `loggy.Fatalf`
- quantize decimals later when you must (never do it early just cuz)
- we like vendoring static web assets using go:embed
- never embed css/js in html (create separate files)

## logging

- `main()` should call `loggy.Init()` to make it how we like it
- use `loggy.Fatalf()` because it raises `SIGINT` so that `main()` can catch and call `coinbaseClient.CancelAllOrders()` if not in dry mode

## decimal library

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

### beware of overflow

The decimal library only supports numbers up to the tens of billions. It will panic if any
computation goes higher than that. Therefore you must choose algorithms that keep the scale
of intermediary computations small. For example, to compute an average, rather than summing
all the numbers and then dividing, consider using a running method like Welford's algorithm.

## time and durations

- `clocky.Time`
- `clocky.Duration`
- Use `clocky.Now()` becasue it can be mocked for backtesting.

## ops

we have an aws z1d instance 1ms away from coinbase in us-east-1 named penny.

we have an aws z1d instance in tokyo named nickel that lets us access binance data 21ms faster via amazon's private internet.

## basis points

- when we say "1 bps" what that means is 0.0001
- the basis point is a convention to make very small percentages readable
- would you do math on an iso8601 timestamp? please don't do it on basis points

## spread backtesting

spread is the main trading strategy in `cmd/spread/`. it uses binance-coinbase spread mean reversion.

## viewing charts

You can see an ASCII chart of what's in any particular dataset, like follows:

```
go run ./cmd/chart -symbol BTC weekend
```

The dataset above is stored in `~/marketdata/weekend/coinbase/BTC-USD` and it contains
a binary-encoded array of `ds/tick.go` structures.

## running backtests

```bash
# single backtest
go run ./cmd/spread -backtest chaos -symbol DOGE

# uber test script (tests many coins across many datasets)
./scripts/test.sh spread -spread 2 -samples 7000 -skew 1 -buygap 5 -target 5000 -comfort 20
```
