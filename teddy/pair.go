package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/binance"
	"dropbear/exchange/coinbase"
	"dropbear/loggy"
	"log"
	"strings"
	"sync"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

type Pair struct {
	Lock           sync.RWMutex
	Exchange       *Exchange
	LastPrice      decimal.Decimal
	BaseCurrency   string // e.g. BTC
	QuoteCurrency  string // e.g. USD
	BaseIncrement  decimal.Decimal
	QuoteIncrement decimal.Decimal
	QuoteMinSize   decimal.Decimal
	QuoteMaxSize   decimal.Decimal
	BaseMinSize    decimal.Decimal
	BaseMaxSize    decimal.Decimal
	OrderBook      *ds.Book
	OnTick         func(*ds.Tick)
	openOrders     *treeset.Set[*Order]
	dataLock       sync.RWMutex
}

func (p *Pair) String() string {
	return p.Symbol()
}

func (p *Pair) Symbol() string {
	switch p.Exchange.Exchange {
	case ds.ExchangeBinance:
		return p.BaseCurrency + p.QuoteCurrency
	default:
		return p.BaseCurrency + "-" + p.QuoteCurrency
	}
}

func newPair(exchange *Exchange, symbol string) *Pair {
	parts := strings.Split(symbol, "-")
	if len(parts) != 2 {
		panic("invalid pair symbol: " + symbol)
	}
	p := &Pair{
		Exchange:       exchange,
		BaseCurrency:   parts[0],
		QuoteCurrency:  parts[1],
		BaseIncrement:  decimal.Satoshi,
		QuoteIncrement: decimal.Satoshi,
		QuoteMinSize:   decimal.One,
		QuoteMaxSize:   decimal.Max,
		BaseMinSize:    decimal.Satoshi,
		BaseMaxSize:    decimal.Max,
		OrderBook:      ds.NewBook(),
		OnTick:         func(*ds.Tick) {},
		openOrders:     treeset.NewWith(compareOrdersByClientOrderID),
	}
	return p
}

func (p *Pair) init() {
	if Live {
		p.refresh()
		go p.refreshDaemon()
		switch p.Exchange.Exchange {
		case ds.ExchangeBinance:
			go p.liveDataDaemon(binance.MarketData(p.Symbol(), BinanceClient))
		case ds.ExchangeCoinbase:
			go p.liveDataDaemon(coinbase.MarketData(p.Symbol(), CoinbaseClient))
		default:
			panic("unsupported exchange")
		}
	} else {
		if p.Exchange.Exchange == ds.ExchangeCoinbase {
			p.Exchange.Holdings.Get(p.BaseCurrency)
			p.Exchange.Holdings.Get(p.QuoteCurrency)
		}
		gManager.Register(p)
	}
}

func (p *Pair) liveDataDaemon(channel <-chan *ds.Tick) {
	for tick := range channel {
		p.handleTick(tick)
		p.OnTick(tick)
	}
}

func (p *Pair) handleTick(tick *ds.Tick) {

	// update last price
	for _, trade := range tick.Trades {
		p.LastPrice = trade.Price
	}

	// update order book
	if tick.Bids != nil || tick.Asks != nil {
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

	// simulate people trading with our algorithm
	if !Live {
		p.Lock.RLock()
		var orders []*Order
		for i := p.openOrders.Iterator(); i.Next(); {
			orders = append(orders, i.Value())
		}
		p.Lock.RUnlock()
		for _, trade := range tick.Trades {
			remaining := trade.Quantity
			for _, order := range orders {
				if !remaining.IsPositive() {
					break
				}
				if order.Type == ds.OrderTypeLimit {
					if trade.Side == order.Side.Flip() {
						// buy fills when trade.Price <= limitPrice
						// sell fills when trade.Price >= limitPrice
						// using side multiplier: limitPrice*side >= tradePrice*side
						if order.LimitPrice.Mul(decimal.Decimal(order.Side)).Cmp(trade.Price.Mul(decimal.Decimal(order.Side))) >= 0 {
							fillValue := order.LimitPrice.Mul(remaining)
							filled, err := order.fill(remaining, fillValue, p.Exchange.MakerFee)
							if err == nil && filled.IsPositive() {
								if gVerbose {
									log.Printf("[sim] %s limit filled %s @ $%s (trade: %s %s @ $%s)",
										order.Side, filled, order.LimitPrice,
										trade.Side, trade.Quantity, trade.Price)
								}
								remaining = remaining.Sub(filled)
							}
						}
					}
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
	default:
		panic("not implemented")
	}
}

func (p *Pair) refreshCoinbase() {
	json, err := CoinbaseClient.GetProduct(p.Symbol())
	if err != nil {
		loggy.Fatalf("failed to get coinbase product info: %v", err)
	}
	p.Lock.Lock()
	defer p.Lock.Unlock()
	p.BaseIncrement = decimal.Parse(json.BaseMinSize)
	p.QuoteIncrement = decimal.Parse(json.QuoteIncrement)
	p.QuoteMinSize = decimal.Parse(json.QuoteMinSize)
	p.QuoteMaxSize = decimal.Parse(json.QuoteMaxSize)
	p.BaseMinSize = decimal.Parse(json.BaseMinSize)
	p.BaseMaxSize = decimal.Parse(json.BaseMaxSize)
}

func (p *Pair) refreshBinance() {
	json, err := BinanceClient.GetSymbol(p.Symbol())
	if err != nil {
		loggy.Fatalf("failed to get binance symbol info: %v", err)
	}
	p.Lock.Lock()
	defer p.Lock.Unlock()
	for _, filter := range json.Filters {
		switch filter.FilterType {
		case "PRICE_FILTER":
			p.QuoteIncrement = decimal.Parse(filter.TickSize)
		case "LOT_SIZE":
			p.BaseIncrement = decimal.Parse(filter.StepSize)
			p.BaseMinSize = decimal.Parse(filter.MinQty)
			p.BaseMaxSize = decimal.Parse(filter.MaxQty)
		case "NOTIONAL":
			p.QuoteMinSize = decimal.Parse(filter.MinNotional)
			p.QuoteMaxSize = decimal.Parse(filter.MaxNotional)
		}
	}
}
