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

```
./scripts/test.sh spread
spread alpha      BTC  vol= 1.8 profit=   34.46 sharpe=  58.30 buys= 62 sells= 49 invested= 1246 good
spread birthday   BTC  vol= 1.5 profit=  -24.20 sharpe= -14.59 buys= 38 sells= 29 invested= 2003 GREAT
spread birthday2  BTC  vol= 1.3 profit=   40.11 sharpe=  13.64 buys= 83 sells= 89 invested= 2244 good
spread bravo      BTC  vol= 1.9 profit=    9.00 sharpe=  67.67 buys= 62 sells= 37 invested= 1326 GREAT
spread chaos      BCH  vol= 0.6 profit=   53.10 sharpe=  38.81 buys= 43 sells= 37 invested=  611 good
spread chaos      BTC  vol= 0.9 profit=   17.66 sharpe=  26.81 buys= 65 sells= 39 invested= 1075 bad
spread chaos      DOGE vol= 1.7 profit=   94.08 sharpe=  40.25 buys=125 sells=120 invested= 1689 good
spread chaos      ETH  vol= 1.2 profit=   34.82 sharpe=  64.89 buys= 75 sells= 54 invested= 1099 good
spread chaos      LTC  vol= 0.5 profit=   27.37 sharpe=  39.89 buys= 30 sells= 20 invested=  718 good
spread chaos      SOL  vol= 1.3 profit=   63.09 sharpe=  38.60 buys= 83 sells= 65 invested= 1596 good
spread chaos      ZEC  vol= 2.0 profit=   23.22 sharpe=   8.86 buys=147 sells=114 invested= 2696 GREAT
spread charlie    BTC  vol= 1.6 profit=   16.34 sharpe=  56.30 buys= 21 sells= 20 invested=  407 bad
spread dogdays    AAVE vol= 0.3 profit=  -11.36 sharpe=  -3.73 buys= 87 sells= 49 invested= 1979 GREAT
spread dogdays    AVAX vol= 0.4 profit=  -35.68 sharpe=  -5.16 buys= 62 sells= 41 invested= 2254 GREAT
spread dogdays    BCH  vol= 0.2 profit=  -33.31 sharpe= -17.91 buys= 47 sells= 45 invested= 1516 GREAT
spread dogdays    BTC  vol= 0.0 profit=  -27.45 sharpe= -18.60 buys=  8 sells=  1 invested= 1095 loco
spread dogdays    DOGE vol= 0.5 profit= -140.78 sharpe= -22.86 buys=117 sells= 75 invested= 3498 loco
spread dogdays    ETH  vol= 0.3 profit=  -56.75 sharpe= -12.47 buys= 58 sells= 34 invested= 2379 GREAT
spread dogdays    ICP  vol= 0.0 profit=    0.00 sharpe=   0.00 buys=  0 sells=  0 invested=    0 GREAT
spread dogdays    LINK vol= 0.7 profit= -103.85 sharpe= -10.25 buys=133 sells= 96 invested= 4531 GREAT
spread dogdays    LTC  vol= 0.3 profit=   -1.30 sharpe=  -0.89 buys= 53 sells= 35 invested= 2057 GREAT
spread dogdays    SOL  vol= 0.4 profit=  -91.70 sharpe= -22.86 buys= 67 sells= 48 invested= 2363 GREAT
spread dogdays    SUI  vol= 0.3 profit=  -78.09 sharpe= -16.80 buys= 60 sells= 34 invested= 2660 GREAT
spread dogdays    TAO  vol= 0.3 profit= -110.53 sharpe= -22.67 buys= 86 sells= 59 invested= 2502 GREAT
spread dogdays    UNI  vol= 0.2 profit=  -22.12 sharpe=  -4.21 buys= 49 sells= 32 invested= 1681 GREAT
spread dogdays    ZEC  vol= 0.9 profit= -212.99 sharpe= -19.04 buys=178 sells=142 invested= 3948 GREAT
spread dreary     AAVE vol= 0.0 profit=    0.00 sharpe=   0.00 buys=  0 sells=  0 invested=    0 GREAT
spread dreary     AVAX vol= 0.2 profit=   18.03 sharpe=  34.95 buys= 13 sells= 10 invested=  344 good
spread dreary     BCH  vol= 0.2 profit=  -20.65 sharpe= -34.20 buys= 29 sells= 13 invested=  758 loco
spread dreary     BTC  vol= 0.4 profit=   13.67 sharpe=  11.70 buys= 38 sells= 24 invested= 1207 bad
spread dreary     DOGE vol= 0.8 profit=   27.45 sharpe=  21.41 buys= 78 sells= 58 invested= 1465 good
spread dreary     ETH  vol= 0.6 profit=   24.15 sharpe=  18.95 buys= 62 sells= 42 invested= 1534 good
spread dreary     ICP  vol= 0.0 profit=    0.00 sharpe=   0.00 buys=  0 sells=  0 invested=    0 bad
spread dreary     LINK vol= 0.8 profit=   39.70 sharpe=  47.39 buys= 78 sells= 64 invested= 1080 good
spread dreary     LTC  vol= 0.4 profit=    2.52 sharpe= -10.35 buys= 30 sells= 26 invested=  768 GREAT
spread dreary     SOL  vol= 0.6 profit=   27.64 sharpe=  28.25 buys= 62 sells= 36 invested= 1339 good
spread dreary     SUI  vol= 0.6 profit=   33.64 sharpe=  23.56 buys= 47 sells= 40 invested=  862 good
spread dreary     TAO  vol= 0.1 profit=   12.39 sharpe=  18.06 buys= 18 sells= 13 invested=  295 bad
spread dreary     UNI  vol= 0.0 profit=    0.00 sharpe=   0.00 buys=  0 sells=  0 invested=    0 bad
spread dreary     ZEC  vol= 2.2 profit=  266.91 sharpe=  74.16 buys=235 sells=190 invested= 1500 good
spread weekend    BCH  vol= 0.3 profit=    0.81 sharpe= -12.58 buys= 44 sells= 23 invested= 1178 GREAT
spread weekend    BTC  vol= 0.2 profit=   -3.99 sharpe= -16.36 buys= 17 sells=  6 invested= 1196 bad
spread weekend    ETH  vol= 0.3 profit=   -6.35 sharpe=  -4.91 buys= 29 sells= 11 invested= 1832 bad
spread weekend    SOL  vol= 0.2 profit=    0.74 sharpe=  -4.78 buys= 16 sells=  7 invested=  978 bad
total n=44 volume=0.75 profit=-26.62 (vs. -403.02) sharpe=+3.99 (vs. -10.48) invested=1489 | GREAT=18 good=14 loco=3 bad=9
```
