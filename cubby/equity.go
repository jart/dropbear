package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/indicators"
	"sync"
)

type Equity struct {
	Symbol     string // e.g. "GOOG", "BRK.B", etc.
	Asset      *alpaca.Asset
	Lock       sync.RWMutex
	Price      decimal.Decimal // current midpoint
	LastPrice  decimal.Decimal // last traded price
	AskPrice   decimal.Decimal // current ask price
	AskSize    decimal.Decimal // current ask size in number of shares
	BidPrice   decimal.Decimal // current bid price
	BidSize    decimal.Decimal // current bid size in number of shares
	Quantity   decimal.Decimal // number of shares held (negative if short)
	Available  decimal.Decimal // number of shares available to trade
	EntryPrice decimal.Decimal // average entry price for current position
	OnCandle   func(*indicators.Candle)
	OnReady    func()
	isReady    bool
}

// Equities holds all added equities by symbol.
var Equities = make(map[string]*Equity)

// AddEquity creates and registers a new Equity with the given symbol.
func AddEquity(symbol string) *Equity {
	if Running {
		panic("cannot create equity while cubby is running")
	}
	e := Equities[symbol]
	if e != nil {
		return e
	}
	asset := alpaca.GetAsset(symbol)
	if asset == nil {
		panic("unknown asset: " + symbol)
	}
	e = &Equity{
		Symbol:   symbol,
		Asset:    asset,
		OnCandle: func(*indicators.Candle) {},
		OnReady:  func() {},
	}
	Equities[symbol] = e
	return e
}

func (e *Equity) String() string {
	return e.Symbol
}
