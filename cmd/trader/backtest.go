package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/symbol"
	"errors"
	"os"
	"time"
)

type Broker interface {
	GetPositions() ([]alpaca.Position, error)
	GetQuotes([]string, alpaca.Feed) (map[string]*sip.Quote, error)
	CreateOrder(*alpaca.CreateOrderRequest) (*alpaca.Order, error)
	CancelOrder(string) error
	CancelAllOrders() error
	SyncAssets() error
}

// Backtest replays a recorded sip file for offline testing.
type Backtest struct {
	StockUpdates chan *sip.Message
	OrderUpdates chan *alpaca.OrderUpdate
	Heartbeat    chan time.Time
	file         *sip.File
	exit         chan os.Signal
	submit       chan *backtestOrder
	cancel       chan string                       // clientOrderID, or "" for cancel all
	pending      []*backtestOrder                  // only accessed by run goroutine
	moc          []*backtestOrder                  // only accessed by run goroutine
	positions    map[symbol.Symbol]decimal.Decimal // only accessed by run goroutine
	lastTrade    map[symbol.Symbol]decimal.Decimal // only accessed by run goroutine
	lastQuote    map[symbol.Symbol]*sip.Quote      // only accessed by run goroutine
	nextBeat     clocky.Time                       // only accessed by run goroutine
	closeTime    clocky.Time                       // only accessed by run goroutine
}

func NewBacktest(path string, exit chan os.Signal) (*Backtest, error) {
	file, err := sip.OpenFile(path)
	if err != nil {
		return nil, err
	}
	b := &Backtest{
		file:         file,
		exit:         exit,
		StockUpdates: make(chan *sip.Message, 1000),
		OrderUpdates: make(chan *alpaca.OrderUpdate, 1000),
		Heartbeat:    make(chan time.Time, 1000),
		submit:       make(chan *backtestOrder, 1000),
		cancel:       make(chan string, 1000),
		positions:    make(map[symbol.Symbol]decimal.Decimal),
		lastTrade:    make(map[symbol.Symbol]decimal.Decimal),
		lastQuote:    make(map[symbol.Symbol]*sip.Quote),
	}
	go b.run()
	return b, nil
}

func (b *Backtest) CreateOrder(body *alpaca.CreateOrderRequest) (*alpaca.Order, error) {
	sym, err := symbol.Parse(body.Symbol)
	if err != nil {
		return nil, err
	}
	exchange := sip.ExchangeNone
	lotSize := cboe.LotSize(body.LimitPrice)
	isOddLotOrder := body.Qty.Cmp(body.Qty.QuantizeTruncate(lotSize)) != 0
	if !isOddLotOrder && body.AdvancedInstructions != nil {
		switch body.AdvancedInstructions.Destination {
		case alpaca.OrderDestinationARCA:
			exchange = sip.ExchangeARCA
		case alpaca.OrderDestinationNASDAQ:
			exchange = sip.ExchangeNASDAQ
		case alpaca.OrderDestinationNYSE:
			exchange = sip.ExchangeNYSE
		}
	}
	b.submit <- &backtestOrder{
		clientOrderID: body.ClientOrderID,
		symbol:        sym,
		exchange:      exchange,
		side:          body.Side,
		qty:           body.Qty,
		price:         body.LimitPrice,
		moc:           body.TimeInForce == alpaca.TimeInForceCLS,
		odd:           isOddLotOrder,
	}
	return nil, nil
}

func (b *Backtest) CancelAllOrders() error {
	b.cancel <- ""
	return nil
}

func (b *Backtest) CancelOrder(id string) error {
	b.cancel <- id
	return nil
}

func (b *Backtest) SyncAssets() error {
	return nil
}

func (b *Backtest) GetQuotes(symbols []string, feed alpaca.Feed) (map[string]*sip.Quote, error) {
	return nil, errors.New("not implemented")
}

func (b *Backtest) GetPositions() ([]alpaca.Position, error) {
	return nil, errors.New("not implemented")
}

type backtestOrder struct {
	clientOrderID string
	exchange      sip.Exchange
	symbol        symbol.Symbol
	side          ds.Side
	qty           decimal.Decimal
	filled        decimal.Decimal
	price         decimal.Decimal
	queueAhead    decimal.Decimal // shares ahead of us in FIFO queue
	odd           bool
	moc           bool
}

func (b *Backtest) run() {
	defer close(b.StockUpdates)
	ts := clocky.Now()
	for {
		msg := b.file.Read()
		if msg == nil {
			break
		}
		if msg.Timestamp.After(ts) {
			clocky.SetNow(msg.Timestamp)
			ts = msg.Timestamp
		}
		if ts >= b.nextBeat {
			b.nextBeat = ts.Add(kHeartbeatInterval)
			select {
			case b.Heartbeat <- time.Time{}:
			default:
			}
		}
		// fill MOC orders at market close
		if b.closeTime != 0 && ts >= b.closeTime {
			b.fillMOC()
			b.closeTime = 0
		}
		// compute close time when we enter a new day
		if b.closeTime == 0 && cboe.GetSession(ts) == cboe.SessionDay {
			year, month, day := ts.Date()
			b.closeTime = cboe.GetCloseTime(year, month, day)
		}
		b.drainSubmits()
		switch msg.Type {
		case sip.MessageTypeTrade:
			t := msg.Trade()
			b.lastTrade[t.Symbol] = t.Price
			b.checkFills(t)
		case sip.MessageTypeQuote:
			q := msg.Quote()
			b.lastQuote[q.Symbol] = q
		}
		b.StockUpdates <- msg
	}
	b.exit <- os.Interrupt
}

func (b *Backtest) drainSubmits() {
	for {
		select {
		case order := <-b.submit:
			if order.moc {
				b.moc = append(b.moc, order)
			} else if b.isMarketable(order) {
				b.fillMarketable(order)
			} else {
				// snapshot queue depth ahead of us for FIFO exchanges
				if order.exchange == sip.ExchangeNASDAQ {
					if q := b.lastQuote[order.symbol]; q != nil {
						switch order.side {
						case ds.SideBuy:
							if order.price.Cmp(q.BidPrice) == 0 {
								order.queueAhead = decimal.FromInt(int(q.BidSize))
							}
						default:
							if order.price.Cmp(q.AskPrice) == 0 {
								order.queueAhead = decimal.FromInt(int(q.AskSize))
							}
						}
					}
				}
				b.pending = append(b.pending, order)
			}
			b.OrderUpdates <- &alpaca.OrderUpdate{
				Event:     alpaca.OrderEventNew,
				Timestamp: clocky.Now(),
				At:        clocky.Now(),
				Order: &alpaca.Order{
					ID:            order.clientOrderID,
					ClientOrderID: order.clientOrderID,
					Symbol:        order.symbol.String(),
					Side:          order.side,
					Qty:           order.qty,
					LimitPrice:    order.price,
					Status:        alpaca.OrderStatusNew,
					Type:          alpaca.OrderTypeLimit,
				},
			}
		case id := <-b.cancel:
			b.cancelPending(id)
		default:
			return
		}
	}
}

func (b *Backtest) cancelPending(id string) {
	now := clocky.Now()
	n := 0
	for _, order := range b.pending {
		if id != "" && order.clientOrderID != id {
			b.pending[n] = order
			n++
			continue
		}
		b.OrderUpdates <- &alpaca.OrderUpdate{
			Event:     alpaca.OrderEventCanceled,
			Timestamp: now,
			At:        now,
			Order: &alpaca.Order{
				ID:            order.clientOrderID,
				ClientOrderID: order.clientOrderID,
				Symbol:        order.symbol.String(),
				Side:          order.side,
				Qty:           order.qty,
				FilledQty:     order.filled,
				LimitPrice:    order.price,
				Status:        alpaca.OrderStatusCanceled,
				Type:          alpaca.OrderTypeLimit,
			},
		}
	}
	b.pending = b.pending[:n]
}

func (b *Backtest) isMarketable(order *backtestOrder) bool {
	q := b.lastQuote[order.symbol]
	if q == nil {
		return false
	}
	switch order.side {
	case ds.SideBuy:
		return order.price.Cmp(q.AskPrice) >= 0 && q.AskPrice.IsPositive()
	default:
		return order.price.Cmp(q.BidPrice) <= 0 && q.BidPrice.IsPositive()
	}
}

func (b *Backtest) fillMarketable(order *backtestOrder) {
	now := clocky.Now()
	q := b.lastQuote[order.symbol]
	var fillPrice decimal.Decimal
	if order.side == ds.SideBuy {
		fillPrice = q.AskPrice
	} else {
		fillPrice = q.BidPrice
	}
	pos := b.positions[order.symbol]
	if order.side == ds.SideBuy {
		pos = pos.Add(order.qty)
	} else {
		pos = pos.Sub(order.qty)
	}
	b.positions[order.symbol] = pos
	b.OrderUpdates <- &alpaca.OrderUpdate{
		Event:       alpaca.OrderEventFill,
		PositionQty: pos,
		Price:       fillPrice,
		Qty:         order.qty,
		Timestamp:   now,
		At:          now,
		Order: &alpaca.Order{
			ID:             order.clientOrderID,
			ClientOrderID:  order.clientOrderID,
			Symbol:         order.symbol.String(),
			Side:           order.side,
			Qty:            order.qty,
			FilledQty:      order.qty,
			FilledAvgPrice: fillPrice,
			LimitPrice:     order.price,
			Status:         alpaca.OrderStatusFilled,
			Type:           alpaca.OrderTypeLimit,
		},
	}
}

func (b *Backtest) fillMOC() {
	now := clocky.Now()
	for _, order := range b.moc {
		fillPrice := b.lastTrade[order.symbol]
		if fillPrice.IsZero() {
			continue // no trade data for this symbol
		}
		pos := b.positions[order.symbol]
		if order.side == ds.SideBuy {
			pos = pos.Add(order.qty)
		} else {
			pos = pos.Sub(order.qty)
		}
		b.positions[order.symbol] = pos
		b.OrderUpdates <- &alpaca.OrderUpdate{
			Event:       alpaca.OrderEventFill,
			PositionQty: pos,
			Price:       fillPrice,
			Qty:         order.qty,
			Timestamp:   now,
			At:          now,
			Order: &alpaca.Order{
				ID:             order.clientOrderID,
				ClientOrderID:  order.clientOrderID,
				Symbol:         order.symbol.String(),
				Side:           order.side,
				Qty:            order.qty,
				FilledQty:      order.qty,
				FilledAvgPrice: fillPrice,
				Status:         alpaca.OrderStatusFilled,
				Type:           alpaca.OrderTypeMarket,
			},
		}
	}
	b.moc = b.moc[:0]
}

func (b *Backtest) checkFills(trade *sip.Trade) {
	now := clocky.Now()
	n := 0
	for _, order := range b.pending {
		if order.symbol != trade.Symbol {
			b.pending[n] = order
			n++
			continue
		}

		// check if trade happened on exchange
		if order.exchange != sip.ExchangeNone && order.exchange != trade.Exchange {
			b.pending[n] = order
			n++
			continue
		}

		// check if trade crosses our limit price
		var crossed bool
		switch order.side {
		case ds.SideBuy:
			crossed = trade.Price.Cmp(order.price) <= 0
		default:
			crossed = trade.Price.Cmp(order.price) >= 0
		}
		if !crossed {
			b.pending[n] = order
			n++
			continue
		}

		// get quote
		q := b.lastQuote[order.symbol]
		if q == nil {
			b.pending[n] = order
			n++
			continue
		}

		// figure out fill quantity
		tradeQty := decimal.FromInt64(trade.Size)
		if b.isMarketable(order) {
			tradeQty = order.qty.Sub(order.filled)
		} else {
			switch order.exchange {
			case sip.ExchangeNone:
				// pfof processors only fill your order when it's bad
				// for you if they think you're an informed market
				// participant. in practice this means they won't fill
				// your order unless it's marketable.
				b.pending[n] = order
				n++
				continue
			case sip.ExchangeARCA, sip.ExchangeNYSE:
				// estimate queue position pro-rata style based on
				// bid/ask size when order isn't inside the spread and
				// is competing for volume with other market makers.
				// since this order was directly routed to the exchange,
				// pro-rata is how arca and nyse work.
				atLevel := false
				var levelSize decimal.Decimal
				if order.side == ds.SideBuy {
					atLevel = order.price.Cmp(q.BidPrice) <= 0
					levelSize = decimal.FromInt(int(q.BidSize))
				} else {
					atLevel = order.price.Cmp(q.AskPrice) >= 0
					levelSize = decimal.FromInt(int(q.AskSize))
				}
				if atLevel && levelSize.IsPositive() {
					// our share of the queue: tradeQty * orderQty / levelSize
					remaining := order.qty.Sub(order.filled)
					tradeQty = tradeQty.Mul(remaining).Div(levelSize).Min(tradeQty)
					tradeQty = tradeQty.Truncate() // QuantizeTruncate(cboe.LotSize(order.price))
				}
			case sip.ExchangeNASDAQ:
				// nasdaq has fifo order matching so we save the
				// bid/ask size of the price level when our order
				// is created. now we subtract from that with each
				// trade that rolls through. once it all gets
				// depleted, we can fill the order.
				if order.queueAhead.IsPositive() {
					order.queueAhead = order.queueAhead.Sub(tradeQty)
					if order.queueAhead.IsPositive() {
						b.pending[n] = order
						n++
						continue
					}
					// queue drained; fill with whatever spilled over
					tradeQty = order.queueAhead.Neg()
					order.queueAhead = decimal.Zero
				}
			default:
				panic("unknown exchange")
			}
		}

		// fill for min(pro-rata trade size, remaining order qty)
		remaining := order.qty.Sub(order.filled)
		fillQty := remaining.Min(tradeQty)
		order.filled = order.filled.Add(fillQty)
		fillPrice := trade.Price

		if fillQty.IsZero() {
			b.pending[n] = order
			n++
			continue
		}
		if fillQty.Truncate().Cmp(fillQty) != 0 {
			panic("non-integer fill quantity")
		}

		// update simulated position
		pos := b.positions[order.symbol]
		if order.side == ds.SideBuy {
			pos = pos.Add(fillQty)
		} else {
			pos = pos.Sub(fillQty)
		}
		b.positions[order.symbol] = pos

		// determine fill vs partial fill
		event := alpaca.OrderEventPartialFill
		status := alpaca.OrderStatusPartiallyFilled
		if order.filled.Cmp(order.qty) >= 0 {
			event = alpaca.OrderEventFill
			status = alpaca.OrderStatusFilled
		}

		b.OrderUpdates <- &alpaca.OrderUpdate{
			Event:       event,
			PositionQty: pos,
			Price:       fillPrice,
			Qty:         fillQty,
			Timestamp:   now,
			At:          now,
			Order: &alpaca.Order{
				ID:             order.clientOrderID,
				ClientOrderID:  order.clientOrderID,
				Symbol:         order.symbol.String(),
				Side:           order.side,
				Qty:            order.qty,
				FilledQty:      order.filled,
				FilledAvgPrice: fillPrice,
				LimitPrice:     order.price,
				Status:         status,
				Type:           alpaca.OrderTypeLimit,
			},
		}

		// keep order if only partially filled
		if status != alpaca.OrderStatusFilled {
			b.pending[n] = order
			n++
		}
	}
	b.pending = b.pending[:n]
}
