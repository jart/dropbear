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

the spread program (`./scripts/test.sh mm`) has this output:

```
mm dogdays    AAVE vol= 0.8 profit=   12.17 sharpe=   0.40 buys= 417 sells= 146 invested= 4391 GREAT
mm dreary     AAVE vol= 0.2 profit=  -22.67 sharpe= -21.82 buys= 137 sells=   8 invested= 2082 brave
mm dogdays    AVAX vol= 1.0 profit=   26.98 sharpe=   2.12 buys= 166 sells= 101 invested= 3113 GREAT
mm dreary     AVAX vol= 0.9 profit=   13.48 sharpe=   5.61 buys=  84 sells=  34 invested= 1626 bad
mm chaos      BCH  vol= 0.5 profit=   56.33 sharpe=  22.53 buys=  49 sells=  11 invested= 1553 good
mm dogdays    BCH  vol= 1.1 profit=   13.70 sharpe=   1.78 buys= 249 sells=  98 invested= 2128 GREAT
mm dreary     BCH  vol= 0.3 profit=    0.35 sharpe=  -0.74 buys=  36 sells=  21 invested= 1363 GREAT
mm weekend    BCH  vol= 1.1 profit=  -50.61 sharpe= -29.70 buys= 119 sells=  62 invested= 2328 GREAT
mm alpha      BTC  vol= 1.7 profit=   40.92 sharpe=  26.79 buys=  76 sells=  61 invested= 3809 good
mm birthday   BTC  vol= 2.6 profit=   -4.54 sharpe= -74.38 buys=  56 sells=  14 invested= 4108 GREAT
mm birthday2  BTC  vol= 1.8 profit=  236.86 sharpe=  21.08 buys= 168 sells=  40 invested= 4055 GREAT
mm bravo      BTC  vol= 1.9 profit=  -14.13 sharpe=  -1.51 buys=  76 sells=  55 invested= 3573 brave
mm chaos      BTC  vol= 0.7 profit=   46.88 sharpe=  31.85 buys=  64 sells=  31 invested= 3744 good
mm charlie    BTC  vol= 3.0 profit=   25.43 sharpe=  72.63 buys=  79 sells=  42 invested= 4344 bad
mm dogdays    BTC  vol= 1.3 profit= -109.01 sharpe= -13.57 buys= 158 sells= 127 invested= 6219 GREAT
mm dreary     BTC  vol= 1.6 profit=   45.19 sharpe=  22.94 buys= 122 sells=  84 invested= 4118 good
mm weekend    BTC  vol= 1.0 profit=   46.56 sharpe=  23.90 buys= 123 sells=  24 invested= 4310 good
mm chaos      DOGE vol= 1.5 profit=   95.27 sharpe=  29.95 buys= 156 sells=  63 invested= 5197 good
mm dogdays    DOGE vol= 2.3 profit=  -92.43 sharpe=  -8.06 buys= 368 sells= 286 invested= 5658 GREAT
mm dreary     DOGE vol= 1.0 profit=  142.59 sharpe=  32.01 buys= 151 sells=  43 invested= 6851 GREAT
mm chaos      ETH  vol= 2.7 profit=   18.14 sharpe=  12.98 buys= 137 sells=  77 invested= 3687 bad
mm dogdays    ETH  vol= 3.0 profit=   11.59 sharpe=   1.09 buys=1176 sells= 316 invested= 6011 GREAT
mm dreary     ETH  vol= 0.9 profit=   59.22 sharpe=  20.01 buys= 297 sells=  26 invested= 4936 good
mm weekend    ETH  vol= 0.7 profit=   32.00 sharpe=  26.40 buys=  54 sells=  41 invested= 3119 good
mm dogdays    LINK vol= 1.9 profit=  -87.79 sharpe=  -8.12 buys= 519 sells= 201 invested= 4783 GREAT
mm dreary     LINK vol= 1.0 profit=   61.75 sharpe=  39.30 buys= 160 sells=  45 invested= 2597 good
mm chaos      LTC  vol= 1.1 profit=   89.90 sharpe=  28.83 buys= 179 sells=  59 invested= 4212 bad
mm dogdays    LTC  vol= 1.3 profit=   56.30 sharpe=   7.65 buys= 387 sells=  85 invested= 4439 GREAT
mm dreary     LTC  vol= 1.5 profit=   63.46 sharpe=   9.35 buys= 256 sells=  29 invested= 4045 GREAT
mm chaos      SOL  vol= 1.8 profit=  103.37 sharpe=  29.48 buys=  86 sells= 161 invested= 5175 good
mm dogdays    SOL  vol= 2.1 profit=   73.57 sharpe=   7.95 buys= 473 sells= 241 invested= 4163 GREAT
mm dreary     SOL  vol= 0.6 profit=   68.52 sharpe=  24.43 buys=  53 sells=  33 invested= 3758 good
mm weekend    SOL  vol= 1.2 profit=   37.21 sharpe=   9.12 buys=  96 sells=  65 invested= 2740 good
mm dogdays    SUI  vol= 1.7 profit= -136.70 sharpe= -13.16 buys= 367 sells= 156 invested= 4859 GREAT
mm dreary     SUI  vol= 0.7 profit=   77.93 sharpe=  17.97 buys=  92 sells=  53 invested= 4484 good
mm dogdays    TAO  vol= 0.7 profit= -268.55 sharpe= -28.39 buys= 230 sells= 126 invested= 3864 GREAT
mm dreary     TAO  vol= 0.8 profit=  159.60 sharpe=  29.99 buys= 116 sells=  57 invested= 1025 good
mm dogdays    UNI  vol= 0.8 profit=  -42.79 sharpe=  -2.61 buys= 227 sells= 128 invested= 5035 GREAT
mm dreary     UNI  vol= 0.4 profit=   30.80 sharpe=  21.39 buys= 275 sells=  25 invested= 1588 good
mm chaos      ZEC  vol= 0.7 profit=    5.73 sharpe=   1.80 buys=  36 sells=  21 invested= 1683 GREAT
mm dogdays    ZEC  vol= 2.3 profit=  -42.89 sharpe=  -3.86 buys= 322 sells= 260 invested= 3702 GREAT
mm dreary     ZEC  vol= 1.6 profit=  759.53 sharpe=  40.44 buys= 132 sells=  52 invested= 7963 bad
total n=42 volume=1.45 profit=+55.41 (vs. -167.85) sharpe=+10.82 (vs. -0.29) invested=3868 | GREAT=20 good=15 brave=2 bad=5
```
