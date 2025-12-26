# dropbear coding guidelines

this project does high frequency trading of crypto.

we have an aws z1d instance 1ms away from coinbase in us-east-1 named penny.

we have an aws z1d instance in tokyo named nickel that lets us access binance data 21ms faster via amazon's private internet.

our goal is to use superior market intelligence and telecommunications to beat the market and pick off market makers.

we believe Binance BTCUSDT futures predict Coinbase BTC-USD by 600ms.

binance trade data comes in the instant it happens.

coinbase has a 50ms heartbeat on l2/trade data.

## commands

- `go fmt ./...`
- `go test ./...`
- never use `go build` (it litters executables) just write a test instead even if it does nothing

## testing

- please write benchmarks too if it makes sense
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

## viewing charts

You can see an ASCII chart of what's in any particular dataset, like follows:

```
go run ./cmd/chart -symbol BTC weekend
# weekend BTC-USD
# 2025-12-20T21:50:57.236458 to 2025-12-21T11:58:41.755981 (14h7m44s519ms523us)
# 88067.03 to 88465.93 (0.45%)
# range: 87601.87 - 89005.99

   89006 |
         |
         |                        **             *
         |                       ****            **
         |                       ***** *        ***
         |                   * * *******    *   ****
         |                   *************************
         |                  **************************
         |                  **************************                         *         *
   88304 |                  ***************************                       **         *
         |                 *****************************                     ******     **
         |                ******************************                  * ********   ***
         |   **        *********************************                 ***********   ***
         |****** ***************************************              *  ***********  ****
         |***********************************************     * **    ********************
         |***********************************************   ******* **********************
         |************************************************  ******************************
         |************************************************  ******************************
         |********************************************************************************
   87602 |********************************************************************************
         +
```

The dataset above is stored in `~/marketdata/weekend/coinbase/BTC-USD` and it contains
a zstd-compressed binary-encoded array of `ds/tick.go` structures.

## running backtests

```bash
# single backtest
go run ./cmd/spread -backtest chaos -symbol DOGE

# uber test script (tests many coins across many datasets)
./scripts/test.sh spread -spread 2 -samples 7000 -skew 1 -buygap 5 -target 5000 -comfort 20
```

# the march of progress

the `spread` program (`./scripts/test.sh spread`) has this output:

```
spread dogdays    AAVE vol=  0.3 profit=  -11.12 sharpe=  -4.00 buys=    99 sells=    51 invested= 1792 GREAT
spread dreary     AAVE vol=  0.0 profit=    0.00 sharpe=   0.00 buys=     0 sells=     0 invested=    0 GREAT
spread dogdays    AVAX vol=  0.4 profit=  -32.41 sharpe=  -4.51 buys=    64 sells=    44 invested= 2328 GREAT
spread dreary     AVAX vol=  0.2 profit=   15.94 sharpe=  38.90 buys=    11 sells=    10 invested=  267 good
spread chaos      BCH  vol=  0.6 profit=   58.35 sharpe=  39.11 buys=    47 sells=    38 invested=  634 good
spread dogdays    BCH  vol=  0.2 profit=  -36.72 sharpe= -19.10 buys=    59 sells=    52 invested= 1603 GREAT
spread dreary     BCH  vol=  0.3 profit=  -20.56 sharpe= -29.53 buys=    42 sells=    21 invested=  928 brave
spread weekend    BCH  vol=  0.4 profit=   -0.05 sharpe= -14.18 buys=    61 sells=    24 invested= 1337 GREAT
spread alpha      BTC  vol=  2.2 profit=   36.34 sharpe=  54.76 buys=    76 sells=    64 invested= 1147 good
spread birthday   BTC  vol=  2.0 profit=  -33.47 sharpe= -14.66 buys=    57 sells=    37 invested= 2737 GREAT
spread birthday2  BTC  vol=  1.9 profit=   49.43 sharpe=  13.14 buys=   127 sells=   138 invested= 2784 good
spread bravo      BTC  vol=  2.2 profit=    9.11 sharpe=  67.11 buys=    76 sells=    45 invested= 1327 GREAT
spread chaos      BTC  vol=  2.1 profit=   19.32 sharpe=  14.50 buys=   126 sells=   108 invested= 1883 bad
spread charlie    BTC  vol=  1.7 profit=   17.31 sharpe=  56.22 buys=    25 sells=    23 invested=  341 bad
spread dogdays    BTC  vol=  0.5 profit=  -48.46 sharpe= -14.58 buys=    91 sells=    54 invested= 2623 GREAT
spread dreary     BTC  vol=  1.3 profit=   18.71 sharpe=   7.30 buys=   124 sells=    87 invested= 1936 bad
spread weekend    BTC  vol=  0.5 profit=   -6.52 sharpe= -13.35 buys=    45 sells=    27 invested= 2454 bad
spread chaos      DOGE vol=  2.2 profit=  108.11 sharpe=  43.10 buys=   153 sells=   154 invested= 2051 good
spread dogdays    DOGE vol=  0.5 profit= -148.66 sharpe= -23.59 buys=   118 sells=    68 invested= 3606 brave
spread dreary     DOGE vol=  1.2 profit=   31.81 sharpe=  21.60 buys=   114 sells=    98 invested= 1608 good
spread chaos      ETH  vol=  2.4 profit=   50.86 sharpe=  59.88 buys=   145 sells=   131 invested= 1528 good
spread dogdays    ETH  vol=  0.4 profit=  -83.91 sharpe= -12.57 buys=    85 sells=    44 invested= 3372 GREAT
spread dreary     ETH  vol=  1.3 profit=   39.71 sharpe=  23.14 buys=   130 sells=    99 invested= 1941 good
spread weekend    ETH  vol=  0.6 profit=   -7.08 sharpe=  -3.62 buys=    52 sells=    29 invested= 2510 bad
spread dogdays    LINK vol=  0.7 profit= -105.14 sharpe= -10.21 buys=   146 sells=   107 invested= 4599 GREAT
spread dreary     LINK vol=  0.9 profit=   46.25 sharpe=  51.44 buys=    84 sells=    75 invested= 1115 good
spread chaos      LTC  vol=  0.5 profit=   25.04 sharpe=  49.09 buys=    36 sells=    21 invested=  651 good
spread dogdays    LTC  vol=  0.5 profit=   -7.77 sharpe=  -2.24 buys=    80 sells=    54 invested= 2412 GREAT
spread dreary     LTC  vol=  0.4 profit=    0.02 sharpe= -12.18 buys=    32 sells=    27 invested=  800 brave
spread chaos      SOL  vol=  1.9 profit=   84.19 sharpe=  46.59 buys=   121 sells=   102 invested= 2030 good
spread dogdays    SOL  vol=  0.6 profit= -101.00 sharpe= -18.68 buys=   110 sells=    81 invested= 2971 GREAT
spread dreary     SOL  vol=  1.0 profit=   34.64 sharpe=  22.23 buys=    97 sells=    54 invested= 1886 good
spread weekend    SOL  vol=  0.4 profit=    2.39 sharpe=  -3.70 buys=    26 sells=    16 invested= 1296 bad
spread dogdays    SUI  vol=  0.3 profit=  -85.93 sharpe= -16.68 buys=    73 sells=    41 invested= 2774 GREAT
spread dreary     SUI  vol=  0.6 profit=   25.80 sharpe=  17.44 buys=    56 sells=    47 invested=  827 good
spread dogdays    TAO  vol=  0.3 profit= -106.31 sharpe= -25.12 buys=    83 sells=    56 invested= 2309 GREAT
spread dreary     TAO  vol=  0.1 profit=   13.06 sharpe=  19.07 buys=    21 sells=    13 invested=  295 bad
spread dogdays    UNI  vol=  0.2 profit=  -16.87 sharpe=  -3.93 buys=    46 sells=    32 invested= 1449 GREAT
spread dreary     UNI  vol=  0.0 profit=    0.00 sharpe=   0.00 buys=     0 sells=     0 invested=    0 bad
spread chaos      ZEC  vol=  2.0 profit=   25.77 sharpe=  10.04 buys=   147 sells=   120 invested= 2629 GREAT
spread dogdays    ZEC  vol=  1.0 profit= -183.01 sharpe= -19.06 buys=   228 sells=   157 invested= 3367 GREAT
spread dreary     ZEC  vol=  2.3 profit=  267.90 sharpe=  73.19 buys=   232 sells=   195 invested= 1546 good
total n=42 volume=0.99 profit=-21.38 (vs. -354.50) sharpe=+3.84 (vs. -10.28) invested=1802 | GREAT=17 good=14 brave=3 bad=8
```
