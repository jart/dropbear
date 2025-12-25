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
main jart@studio:~/dropbear$ ./scripts/test.sh spread -intensity 5m
spread dreary     ICP  vol= 0.00 profit=    0.00 sharpe=-100.00 buys=  0 sells=  0 invested=    0 bad
spread dreary     AVAX vol= 0.15 profit=   34.59 sharpe=  18.12 buys= 21 sells=  0 invested= 1120 good
spread dreary     UNI  vol= 0.00 profit=    0.00 sharpe=-100.00 buys=  0 sells=  0 invested=    0 bad
spread dreary     AAVE vol= 0.00 profit=    0.00 sharpe=-100.00 buys=  0 sells=  0 invested=    0 loco
spread dreary     LTC  vol= 0.49 profit=  -27.04 sharpe=  -4.39 buys= 65 sells=  0 invested= 4787 loco
spread weekend    BCH  vol= 0.32 profit=    7.48 sharpe=   2.45 buys= 56 sells=  0 invested= 2845 GREAT
spread dreary     TAO  vol= 0.15 profit=   10.08 sharpe=   2.71 buys= 37 sells=  0 invested= 1016 bad
spread chaos      BCH  vol= 0.32 profit=   71.67 sharpe=  23.94 buys= 44 sells=  0 invested= 1810 good
spread chaos      LTC  vol= 0.31 profit=   51.03 sharpe=  15.15 buys= 32 sells=  0 invested= 2176 bad
spread dogdays    AVAX vol= 0.39 profit= -233.70 sharpe=  -7.04 buys=106 sells=  0 invested= 8313 GREAT
spread dogdays    ICP  vol= 0.00 profit=    0.00 sharpe=-100.00 buys=  0 sells=  0 invested=    0 loco
spread dreary     BCH  vol= 0.42 profit=  -63.13 sharpe= -11.07 buys=100 sells=  0 invested= 4440 GREAT
spread dogdays    UNI  vol= 0.22 profit= -132.19 sharpe=  -7.12 buys= 75 sells=  0 invested= 3922 GREAT
spread dreary     LINK vol= 0.54 profit=  112.55 sharpe=  11.89 buys= 92 sells=  0 invested= 5788 GREAT
spread dreary     SUI  vol= 0.50 profit=   85.39 sharpe=   8.49 buys= 83 sells=  0 invested= 5741 good
spread dogdays    LTC  vol= 0.37 profit= -137.39 sharpe=  -4.92 buys=108 sells=  0 invested=10038 GREAT
spread dogdays    TAO  vol= 0.34 profit= -439.17 sharpe= -14.01 buys=152 sells=  0 invested= 6467 GREAT
spread chaos      DOGE vol= 0.92 profit=  168.00 sharpe=  12.80 buys=125 sells=  0 invested= 8586 bad
spread weekend    SOL  vol= 0.71 profit=   51.67 sharpe=   7.96 buys= 93 sells=  0 invested= 7241 good
spread dreary     DOGE vol= 0.69 profit=  140.43 sharpe=  10.44 buys=119 sells=  0 invested=11206 GREAT
spread dogdays    BCH  vol= 0.34 profit= -176.20 sharpe=  -9.10 buys=146 sells=  0 invested= 7989 GREAT
spread dogdays    AAVE vol= 0.29 profit= -181.22 sharpe=  -6.07 buys=138 sells=  0 invested= 7592 GREAT
spread dreary     SOL  vol= 0.59 profit=  100.40 sharpe=  10.92 buys= 95 sells=  0 invested= 7340 bad
spread dogdays    SUI  vol= 0.49 profit= -465.78 sharpe= -10.70 buys=186 sells=  0 invested=13314 GREAT
spread chaos      SOL  vol= 0.73 profit=  106.61 sharpe=  11.92 buys= 77 sells=  0 invested= 7139 bad
spread dogdays    LINK vol= 0.60 profit= -557.73 sharpe= -10.52 buys=215 sells=  0 invested=18646 GREAT
spread chaos      ZEC  vol= 0.94 profit=  -15.41 sharpe=  -0.50 buys=117 sells=  0 invested= 9985 GREAT
spread dogdays    DOGE vol= 0.55 profit= -704.54 sharpe= -16.14 buys=207 sells=  0 invested=15546 GREAT
spread dogdays    SOL  vol= 0.49 profit= -616.13 sharpe= -15.99 buys=162 sells=  0 invested=15077 GREAT
spread charlie    BTC  vol= 2.26 profit=   54.95 sharpe=  27.72 buys= 55 sells=  0 invested= 4780 bad
spread weekend    ETH  vol= 0.84 profit=   -3.62 sharpe=   0.58 buys= 89 sells=  0 invested= 8691 bad
spread dreary     ZEC  vol= 1.11 profit= 1368.54 sharpe=  33.61 buys=224 sells=  0 invested=15473 bad
spread bravo      BTC  vol= 1.63 profit=  -41.83 sharpe= -14.33 buys= 83 sells=  0 invested= 5534 loco
spread dogdays    ZEC  vol= 0.82 profit=-1718.31 sharpe= -18.13 buys=257 sells=  0 invested=26522 good
spread weekend    BTC  vol= 0.78 profit=  -12.80 sharpe=  -2.03 buys= 91 sells=  0 invested= 8122 bad
spread dreary     ETH  vol= 0.59 profit=  107.86 sharpe=  11.02 buys= 92 sells=  0 invested= 8280 GREAT
spread alpha      BTC  vol= 1.48 profit=  108.04 sharpe=  19.58 buys= 90 sells=  0 invested= 6263 good
spread chaos      ETH  vol= 0.73 profit=   -0.37 sharpe=  -0.32 buys= 78 sells=  0 invested= 5589 bad
spread birthday   BTC  vol= 2.41 profit= -102.42 sharpe= -37.34 buys=104 sells=  0 invested= 6268 GREAT
spread dreary     BTC  vol= 0.67 profit=  125.88 sharpe=  11.84 buys=120 sells=  0 invested=10760 good
spread chaos      BTC  vol= 0.84 profit=  -37.96 sharpe=  -5.35 buys=100 sells=  0 invested= 4394 bad
spread dogdays    ETH  vol= 0.60 profit= -373.43 sharpe=  -8.36 buys=186 sells=  0 invested=18175 GREAT
spread birthday2  BTC  vol= 1.09 profit=  571.66 sharpe=  37.83 buys=161 sells=  0 invested=13392 GREAT
spread dogdays    BTC  vol= 0.52 profit= -331.91 sharpe= -10.01 buys=162 sells=  0 invested=18592 GREAT
total n=44 volume=0.64 profit=-70.35 (vs. -168.34) sharpe=-7.60 (vs. -1.57) invested=7931 | GREAT=20 good=7 bad=13
```
