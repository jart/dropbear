# dropbear coding guidelines

this project does high frequency trading of crypto.

we have an aws z1d instance 1ms away from coinbase in us-east-1 named penny.

we have an aws z1d instance in tokyo named nickel that lets us access binance data 21ms faster via amazon's private internet.

our goal is to use superior market intelligence and telecommunications to beat the market and pick off market makers.

we believe Binance BTCFDUSD trades predict Coinbase BTC-USD by 600ms.

binance trade data comes in the instant it happens.

coinbase has a 50ms heartbeat on l2/trade data.

## commands

- `go fmt ./...`
- `go test ./...`
- never use `go build` (it litters executables) just write a test instead even if it does nothing

## testing

- please write benchmarks too if it makes sense
- table-driven tests are nice (e.g. `decimal/decimal_test.go`)
- use deterministic random seeds for reproducibility
- we'd like to be more formal and safe than we are
- try to tease out edge cases and corner cases

## packages

- `teddy/` is our QuantConnect-like framework for writing trading algorithms
- `cmd/spread/` is an example of a trading bot that uses the teddy framework
- `loggy/` is our logging utilities
- `decimal/` our fixed point number library with 9 decimal places
- `db/` use `db.Get()` to get a WAL2 SQLite singleton into `~/.dropbear.sqlite3`
- `indicators/` has indicators similar quantconnect but better, defines candles
- `orderbook/` uses gods v2 tree set for fast level2 order book management
- `exchange/coinbase/` our own client library (where we currently do our trading!)
- `exchange/binance/` our own client library (used for market data only currently)
- `exchange/alpaca/` our own client library (not doing much with alpaca right now)
- `auth/` lets the dropbear https server support yubikey authentication

## data locations

- `~/.dropbear.sqlite3` - main database for fills/trades
- `~/marketdata/<dataset>/{coinbase,binance}/<SYMBOL>` - zstd-compressed binary market data (new format)

## programs

- `cmd/spread/` is the main trading strategy (live and backtest)
- `cmd/chart/` generates ASCII price charts from market data
- `cmd/dumptick ~/marketdata/weekend/binance/FDUSDUSDT` dumps raw recorded data
- `cmd/record.{binance,coinbase}/` records live websocket data to binary format

## style

- we want algorithms to have optimal time complexity
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
- `d.Quantize({base,quote}Increment)` if you don't care
- `d.Quantize{Nearest,Up,Down,Floor,Ceil}({base,quote}Increment)` when it matters
- `d.Int()` and `d.Int64()` exist, also Sqr, Exp, Neg, Abs, IsZero, IsPositive, etc.

## time and durations

- `clocky.Time`
- `clocky.Duration`
- Use `clocky.Now()` becasue it can be mocked for backtesting.

## basis points

- when we say "1 bps" what that means is 0.0001
- the basis point is a convention to make very small percentages readable
- would you do math on an iso8601 timestamp? please don't do it on basis points

## spread backtesting

spread is the main trading strategy in `cmd/spread/`. it uses binance-coinbase spread mean reversion.

### running backtests

```bash
# single backtest
go run ./cmd/spread -backtest chaos -symbol DOGE

# uber test script (tests many coins across many datasets)
./scripts/test.sh spread -spread 2 -samples 7000 -skew 1 -buygap 5 -target 5000 -comfort 20
```

# the march of progress

## latest status

```
main jart@studio:~/dropbear$ ./scripts/test.sh spread
spread chaos      BCH  vol= 1.06 profit=   77.99 sharpe=  41.38 buys= 80 sells= 81 invested= 1093 good
spread chaos      LTC  vol= 0.79 profit=   33.85 sharpe=  36.00 buys= 47 sells= 38 invested=  867 good
spread chaos      SOL  vol= 3.26 profit=  132.90 sharpe=  44.47 buys=200 sells=167 invested= 2185 good
spread chaos      DOGE vol= 3.38 profit=  143.11 sharpe=  51.22 buys=232 sells=226 invested= 2030 good
spread chaos      ZEC  vol= 3.47 profit=   31.84 sharpe=   5.82 buys=242 sells=196 invested= 3316 GREAT
spread weekend    BCH  vol= 0.67 profit=    2.48 sharpe=   0.59 buys= 76 sells= 56 invested= 1875 GREAT
spread weekend    SOL  vol= 0.69 profit=    2.22 sharpe=   0.89 buys= 49 sells= 37 invested= 1995 bad
spread bravo      BTC  vol= 1.87 profit=    4.52 sharpe=   6.26 buys= 63 sells= 43 invested= 1470 GREAT
spread alpha      BTC  vol= 1.93 profit=   30.07 sharpe=  34.36 buys= 63 sells= 67 invested= 1419 good
spread birthday   BTC  vol= 2.95 profit=  -43.19 sharpe= -26.37 buys= 70 sells= 50 invested= 3330 GREAT
spread charlie    BTC  vol= 2.02 profit=   16.36 sharpe=  53.49 buys= 25 sells= 32 invested=  851 good
spread weekend    ETH  vol= 0.46 profit=   -5.34 sharpe=  -2.31 buys= 35 sells= 19 invested= 2116 bad
spread chaos      ETH  vol= 2.99 profit=   90.48 sharpe=  34.13 buys=186 sells=169 invested= 1566 good
spread chaos      BTC  vol= 1.75 profit=   40.39 sharpe=  20.96 buys=122 sells= 88 invested= 1349 good
spread weekend    BTC  vol= 0.37 profit=   -2.47 sharpe=  -3.93 buys= 23 sells= 16 invested= 1395 bad
spread birthday2  BTC  vol= 1.74 profit=   63.26 sharpe=  14.16 buys=111 sells=123 invested= 3419 good
total n=16 volume=1.84 profit=+38.65 (vs. +61.04) sharpe=+19.44 (vs. +2.64) invested=1892 | GREAT=4 good=9 bad=3
```
