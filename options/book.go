package options

import (
	"dropbear/broker/databento"
	"dropbear/decimal"
	"fmt"
)

type Level struct {
	Price decimal.Decimal
	Size  decimal.Decimal
}

type Levels struct {
	Levels []Level
	Last   Level
}

func (l *Levels) NBBO() (decimal.Decimal, decimal.Decimal) {
	if len(l.Levels) == 0 {
		return decimal.Zero, decimal.Zero
	}
	return l.Levels[len(l.Levels)-1].Price, l.Levels[len(l.Levels)-1].Size
}

func (l *Levels) EstimateFillPrice(shares decimal.Decimal) decimal.Decimal {
	if shares.IsZero() || len(l.Levels) == 0 {
		return decimal.Zero
	}
	remaining := shares
	notional := decimal.Zero // sum of price * size consumed
	for i := len(l.Levels) - 1; i >= 0; i-- {
		lvl := l.Levels[i]
		take := min(lvl.Size, remaining)
		notional = notional.Add(lvl.Price.Mul(take))
		remaining = remaining.Sub(take)
		if remaining.IsZero() {
			return notional.Div(shares)
		}
	}
	// not enough stashed depth to fill the full size
	return decimal.Zero
}

func (b *Levels) Update(dbPrice int64, size decimal.Decimal, bid bool) {
	if dbPrice == databento.UndefPrice {
		b.Levels = nil
		return
	}
	if size.IsZero() {
		panic("size was zero")
	}
	price := decimal.Decimal(dbPrice / 1000)
	if price.Cmp(b.Last.Price) == 0 && size.Cmp(b.Last.Size) == 0 {
		return
	}
	b.Last = Level{Price: price, Size: size}
	if bid {
		for len(b.Levels) > 0 && price.Cmp(b.Levels[len(b.Levels)-1].Price) < 0 {
			b.Levels = b.Levels[:len(b.Levels)-1] // pop
		}
	} else {
		for len(b.Levels) > 0 && price.Cmp(b.Levels[len(b.Levels)-1].Price) > 0 {
			b.Levels = b.Levels[:len(b.Levels)-1] // pop
		}
	}
	if len(b.Levels) > 0 && b.Levels[len(b.Levels)-1].Price.Cmp(price) == 0 {
		b.Levels[len(b.Levels)-1].Size = size // set
		return
	}
	b.Levels = append(b.Levels, Level{Price: price, Size: size}) // push
}

type Book struct {
	Bids Levels
	Asks Levels
}

func (b *Book) UpdateMBP1(m *databento.MBP1) bool {
	switch m.Action {
	case databento.ActionClear:
		b.Bids.Levels = nil
		b.Asks.Levels = nil
		return true
	case databento.ActionTrade:
		return false
	case databento.ActionAdd, databento.ActionModify, databento.ActionCancel:
		b.Bids.Update(m.Levels[0].BidPx, decimal.FromInt(int(m.Levels[0].BidSz)), true)
		b.Asks.Update(m.Levels[0].AskPx, decimal.FromInt(int(m.Levels[0].AskSz)), false)
		return m.Flags&databento.FlagSetLast != 0
	default:
		panic(fmt.Sprintf("invalid action: %v", m.Action))
	}
}
