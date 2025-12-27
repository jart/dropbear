package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/binance"
	"dropbear/exchange/binanceusd"
	"dropbear/exchange/coinbase"
	"dropbear/loggy"
	"strings"
	"sync"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

type Pair struct {
	Symbol         string
	Lock           sync.RWMutex
	Exchange       *Exchange
	LastPrice      decimal.Decimal
	BaseCurrency   *Holding
	QuoteCurrency  *Holding
	BaseIncrement  decimal.Decimal
	QuoteIncrement decimal.Decimal
	QuoteMinSize   decimal.Decimal
	QuoteMaxSize   decimal.Decimal
	BaseMinSize    decimal.Decimal
	BaseMaxSize    decimal.Decimal
	Trades         map[ds.Side]int
	OrderBook      *ds.Book
	OnTick         func(*ds.Tick)
	OnReady        func()
	repr           string
	isReady        bool
	hasL2Data      bool
	hasTradeData   bool
	openOrders     *treeset.Set[*Order]
	dataLock       sync.RWMutex
}

func newPair(exchange *Exchange, symbol string) *Pair {
	if gRunning {
		loggy.Fatalf("cannot create new pair %s@%s while teddy is running", symbol, exchange)
	}
	baseCurrency, quoteCurrency := splitProductID(symbol)
	p := &Pair{
		Symbol:         symbol,
		Exchange:       exchange,
		BaseCurrency:   exchange.Holdings.Get(baseCurrency),
		QuoteCurrency:  exchange.Holdings.Get(quoteCurrency),
		BaseIncrement:  guessIncrement(baseCurrency),
		QuoteIncrement: guessIncrement(quoteCurrency),
		QuoteMinSize:   guessQuoteMinSize(quoteCurrency),
		QuoteMaxSize:   guessQuoteMaxSize(quoteCurrency),
		BaseMinSize:    guessBaseMinSize(baseCurrency),
		BaseMaxSize:    guessBaseMaxSize(baseCurrency),
		Trades:         make(map[ds.Side]int),
		OrderBook:      ds.NewBook(),
		OnTick:         func(*ds.Tick) {},
		OnReady:        func() {},
		openOrders:     treeset.NewWith(compareOrdersByClientOrderID),
		repr:           symbol + "@" + exchange.Exchange.String(),
	}
	if Live {
		p.refresh()
	}
	return p
}

func (p *Pair) String() string {
	return p.repr
}

func (p *Pair) run() {
	go p.refreshDaemon()
	switch p.Exchange.Exchange {
	case ds.ExchangeBinance:
		go p.liveDataDaemon(binance.MarketData(p.Symbol, BinanceClient))
	case ds.ExchangeBinanceusd:
		go p.liveDataDaemon(binanceusd.MarketData(p.Symbol, BinanceusdClient))
	case ds.ExchangeCoinbase:
		go p.liveDataDaemon(coinbase.MarketData(p.Symbol, CoinbaseClient))
	default:
		panic("unsupported exchange")
	}
}

func (p *Pair) liveDataDaemon(channel <-chan *ds.Tick) {
	for tick := range channel {
		p.handleTick(tick)
		p.OnTick(tick)
	}
}

func (p *Pair) handleTick(tick *ds.Tick) {

	// pulse rate limiter
	if Paper {
		gRateLimiter.Pulse(tick.Time)
	}

	// update last price
	if len(tick.Trades) > 0 {
		p.hasTradeData = true
		for _, trade := range tick.Trades {
			p.LastPrice = trade.Price
		}
	}

	// update order book
	if tick.Bids != nil || tick.Asks != nil {
		p.hasL2Data = true
		p.OrderBook.Lock.Lock()
		if tick.Snap {
			p.OrderBook.Clear()
		}
		for _, bid := range tick.Bids {
			p.OrderBook.UpdateBid(bid.Price, bid.Size)
		}
		for _, ask := range tick.Asks {
			p.OrderBook.UpdateAsk(ask.Price, ask.Size)
		}
		p.OrderBook.Broadcast()
		p.OrderBook.Lock.Unlock()
	}

	if !p.isReady && p.hasL2Data && p.hasTradeData {
		p.isReady = true
		p.OnReady()
		p.Exchange.Pairs.markReady(p)
	}

	// simulate passive order fills BEFORE applying book updates
	// trades consume liquidity from the order book; our passive orders
	// only get filled after the liquidity ahead of us is consumed
	if Paper && p.isReady && len(tick.Trades) > 0 {
		p.Lock.RLock()
		var orders []*Order
		for i := p.openOrders.Iterator(); i.Next(); {
			orders = append(orders, i.Value())
		}
		p.Lock.RUnlock()
		for _, trade := range tick.Trades {
			// pop from book to consume liquidity ahead of our orders
			// trade.Side is the taker side, so we pop from the opposite side
			// (a taker sell hits bids, a taker buy lifts asks)
			remaining := trade.Quantity
			for remaining.IsPositive() {
				fill, ok := p.OrderBook.Pop(trade.Side, remaining, trade.Price)
				if !ok {
					break
				}
				remaining = remaining.Sub(fill.Size)
				// check if any of our orders are at this price level
				for _, order := range orders {
					if order.Type != ds.OrderTypeLimit || order.State.IsFinal() {
						continue
					}
					// our bid fills when taker sells, our ask fills when taker buys
					if trade.Side != order.Side.Flip() {
						continue
					}
					// check if trade reached our price level
					// buy fills when fill.Price <= limitPrice
					// sell fills when fill.Price >= limitPrice
					if order.LimitPrice.Mul(decimal.Decimal(order.Side)).Cmp(fill.Price.Mul(decimal.Decimal(order.Side))) < 0 {
						continue
					}
					// we're at this price level - fill our order
					order.Lock.RLock()
					unfilled := order.Quantity.Sub(order.Filled)
					order.Lock.RUnlock()
					fillQty := fill.Size.Min(unfilled)
					if fillQty.IsPositive() {
						fillValue := order.LimitPrice.Mul(fillQty)
						filled, err := order.fill(fillQty, fillValue, p.Exchange.MakerFee, false)
						if err == nil && filled.IsPositive() {
							leftover := fill.Size.Sub(filled)
							if leftover.IsPositive() {
								p.OrderBook.Push(trade.Side.Flip(), fill.Price, leftover)
							}
						}
					}
					break // only one order per price level gets filled per pop
				}
			}
		}
	}

}

func (p *Pair) refreshDaemon() {
	for {
		Hibernate()
		p.refresh()
	}
}

func (p *Pair) refresh() {
	switch p.Exchange.Exchange {
	case ds.ExchangeCoinbase:
		p.refreshCoinbase()
	case ds.ExchangeBinance:
		p.refreshBinance()
	case ds.ExchangeBinanceusd:
		p.refreshBinanceusd()
	default:
		panic("not implemented")
	}
}

func (p *Pair) refreshCoinbase() {
	json, err := CoinbaseClient.GetProduct(p.Symbol)
	if err != nil {
		loggy.Fatalf("failed to get coinbase product info: %v", err)
	}
	p.BaseIncrement.Store(decimal.Parse(json.BaseMinSize))
	p.QuoteIncrement.Store(decimal.Parse(json.QuoteIncrement))
	p.QuoteMinSize.Store(decimal.Parse(json.QuoteMinSize))
	p.QuoteMaxSize.Store(decimal.Parse(json.QuoteMaxSize))
	p.BaseMinSize.Store(decimal.Parse(json.BaseMinSize))
	p.BaseMaxSize.Store(decimal.Parse(json.BaseMaxSize))
}

func (p *Pair) refreshBinance() {
	json, err := BinanceClient.GetSymbol(p.Symbol)
	if err != nil {
		loggy.Fatalf("failed to get binance symbol info: %v", err)
	}
	for _, filter := range json.Filters {
		p.refreshBinanceFilter(filter)
	}
}

func (p *Pair) refreshBinanceusd() {
	json := getBinanceusdSymbol(p.Symbol)
	for _, filter := range json.Filters {
		p.refreshBinanceFilter(filter)
	}
}

func (p *Pair) refreshBinanceFilter(filter *binance.Filter) {
	switch filter.FilterType {
	case "PRICE_FILTER":
		p.QuoteIncrement.Store(decimal.Parse(filter.TickSize))
	case "LOT_SIZE":
		p.BaseIncrement.Store(decimal.Parse(filter.StepSize))
		p.BaseMinSize.Store(decimal.Parse(filter.MinQty))
		p.BaseMaxSize.Store(decimal.Parse(filter.MaxQty))
	case "NOTIONAL":
		p.QuoteMinSize.Store(decimal.Parse(filter.MinNotional))
		p.QuoteMaxSize.Store(decimal.Parse(filter.MaxNotional))
	}
}

var (
	binanceusdExchangeInfoOnce sync.Once
	binanceusdExchangeInfoSave *binanceusd.ExchangeInfo
)

func getBinanceusdExchangeInfo() *binanceusd.ExchangeInfo {
	binanceusdExchangeInfoOnce.Do(func() {
		info, err := BinanceusdClient.GetExchangeInfo()
		if err != nil {
			loggy.Fatalf("failed to get binanceusd exchange info: %v", err)
		}
		binanceusdExchangeInfoSave = info
	})
	return binanceusdExchangeInfoSave
}

func getBinanceusdSymbol(symbol string) *binanceusd.Symbol {
	info := getBinanceusdExchangeInfo()
	for _, s := range info.Symbols {
		if s.Symbol == symbol {
			return s
		}
	}
	return nil
}

func splitProductID(s string) (baseCurrency, quoteCurrency string) {
	if strings.Contains(s, "-") {
		parts := strings.SplitN(s, "-", 2)
		return parts[0], parts[1]
	}
	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		return parts[0], parts[1]
	}
	for _, quote := range binanceusd.QuoteCurrencies {
		if before, found := strings.CutSuffix(s, quote); found && before != "" {
			return before, quote
		}
	}
	panic("invalid product id: " + s)
}

func guessIncrement(currency string) decimal.Decimal {
	switch currency {
	case "USD", "EUR", "GBP", "JPY", "USDT", "USDC", "BUSD", "DAI":
		return decimal.Cent
	default:
		return decimal.Satoshi
	}
}

func guessQuoteMinSize(currency string) decimal.Decimal {
	switch currency {
	case "BTC":
		return decimal.Parse("0.00001")
	case "ETH":
		return decimal.Parse("0.00022")
	default:
		return decimal.One
	}
}

func guessQuoteMaxSize(currency string) decimal.Decimal {
	switch currency {
	case "BTC":
		return decimal.FromInt(200)
	case "ETH":
		return decimal.FromInt(2500)
	default:
		return decimal.FromInt(150_000_000)
	}
}

func guessBaseMinSize(currency string) decimal.Decimal {
	switch currency {
	case "BTC":
		return decimal.Satoshi
	case "ETH":
		return decimal.Parse("0.0001")
	default:
		return decimal.One
	}
}

func guessBaseMaxSize(currency string) decimal.Decimal {
	switch currency {
	case "BTC":
		return decimal.FromInt(3_400)
	case "ETH":
		return decimal.FromInt(508_300)
	default:
		return decimal.FromInt(150_000_000)
	}
}

func comparePairs(p, q *Pair) int {
	if p.Exchange.Exchange < q.Exchange.Exchange {
		return -1
	}
	if p.Exchange.Exchange > q.Exchange.Exchange {
		return +1
	}
	return strings.Compare(p.Symbol, q.Symbol)
}
