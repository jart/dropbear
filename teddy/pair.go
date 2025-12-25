package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/binance"
	"dropbear/exchange/coinbase"
	"dropbear/loggy"
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
		type phil struct {
			o *Order
			q decimal.Decimal
			v decimal.Decimal
		}
		p.Lock.RLock()
		var phils []phil
		for _, trade := range tick.Trades {
			for i := p.openOrders.Iterator(); i.Next(); {
				order := i.Value()
				if order.Type == ds.OrderTypeLimit {
					if trade.Side == order.Side.Flip() {
						if trade.Price.Mul(decimal.Decimal(trade.Side)).Cmp(order.LimitPrice) >= 0 {
							unfilledQuantity := order.Quantity.Sub(order.Filled)
							fillQuantity := unfilledQuantity.Min(trade.Quantity)
							fillValue := order.LimitPrice.Mul(fillQuantity)
							phils = append(phils, phil{order, fillQuantity, fillValue})
						}
					}
				}
			}
		}
		p.Lock.RUnlock()
		for _, f := range phils {
			f.o.fill(f.q, f.v, p.Exchange.MakerFee)
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
			if filter.TickSize != "" {
				p.QuoteIncrement = decimal.Parse(filter.TickSize)
			}
		case "LOT_SIZE":
			if filter.StepSize != "" {
				p.BaseIncrement = decimal.Parse(filter.StepSize)
			}
			if filter.MinQty != "" {
				p.BaseMinSize = decimal.Parse(filter.MinQty)
			}
			if filter.MaxQty != "" {
				p.BaseMaxSize = decimal.Parse(filter.MaxQty)
			}
		case "NOTIONAL":
			if filter.MinNotional != "" {
				p.QuoteMinSize = decimal.Parse(filter.MinNotional)
			}
			if filter.MaxNotional != "" {
				p.QuoteMaxSize = decimal.Parse(filter.MaxNotional)
			}
		}
	}
}
