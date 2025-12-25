# dropbear for humans

harvesting money off crypto markets is like trying to milk a male tiger.
dropbear is one of the few pieces of software that's capable of doing it.
something this good usually isn't open source. for example, quantconnect
is open source, but their min/max indicators are quadratic. hummingbot is
open sauce, but their intensity indicator is cubic. dropbear does these
things in constant time, because computer science is the true faith.

## 1. brokerage accounts

### coinbase

You need to sign up with coinbase.
Please do it by clicking <https://advanced.coinbase.com/join/L8EN839> so we get money for referring you.
It's *very important* that you **do not** buy any cryptocurrency using the Coinbase website, because they
charge normies a 2.5% commission. It's 10x less expensive to let dropbear buy coins for you, through their
advanced trading API. If you run dropbear a few weeks, you'll be paying 100x less commissions and they'll
literally give you a black american express card for free with 4% cash back in bitcoin, I kid you not.

Next, you go on the <https://portal.cdp.coinbase.com/> website and generate an ed25519 API key. You
then need to put the key into a `coinbase.json` in this folder that looks like this:

```
{
   "id": "66fa52e3-7004-4930-94ea-d7c8c9cce52b",
   "privateKey": "hehrcueoadgchudgceodcgaudgcao=="
}
```

## 2. market data

dropbear lets you simulate trading 100% locally. You can strace the process and you'll never see the word "socket".
You don't have to pay us anything for our services. But you do need lots of market data to test before going live.

Just download <https://drive.google.com/drive/folders/1qKdvshCNLjFuFQ_8LCMiKEQH-G8jT4ZF?usp=sharing> as your `~/marketdata` folder.

Alternatively, you can record your own market data as follows:

```
go run ./cmd/record.coinbase -name dreary BTC-USD ETH-USD SOL-USD BCH-USD ZEC-USD USDT-USD DOGE-USD LTC-USD LINK-USD AAVE-USD SUI-USD TAO-USD ICP-USD AVAX-USD UNI-USD
go run ./cmd/record.binance -name dreary FDUSDUSDT USDTUSD BTCFDUSD ETHFDUSD SOLFDUSD BCHFDUSD ZECUSDT DOGEFDUSD LTCFDUSD LINKFDUSD AAVEFDUSD SUIFDUSD TAOFDUSD ICPFDUSD AVAXFDUSD UNIFDUSD
```

## 3. running the software

Here's an example of how to backtest the [cmd/spread/main.go] trading bot.

```
go run ./cmd/spread -backtest chaos -symbol ZEC
```

To run *all* the backtests for the trading bot just say:

```
./scripts/test.sh spread
```

Now you're ready to read [CLAUDE.md](CLAUDE.md) for the rest of our tutorial.
