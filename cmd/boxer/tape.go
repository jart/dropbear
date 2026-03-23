package main

import (
	"dropbear/broker/databento"
	"dropbear/decimal"
	"dropbear/ds/options"
	"log"
)

func onFutureDef(key databento.ApiKey, ticks chan<- *databento.MBP1, f *Future) {
	gFuturesByID[f.ID] = f
	switch f.Symbol {
	case esSymbol:
		gES = f
	case sr1Symbol:
		gSR1 = f
		go fetchFuturePrice(key, gSR1, ticks)
	default:
		log.Fatalf("unknown future symbol: %s", f.Symbol)
	}
}

func onFutureTick(t *databento.MBP1) {
	f := gFuturesByID[t.Header.InstrumentID]
	if f == nil {
		return
	}
	if t.Header.TSEvent > f.TS {
		f.TS = t.Header.TSEvent
		f.Bid = dbnPrice(t.Levels[0].BidPx)
		f.Ask = dbnPrice(t.Levels[0].AskPx)
		f.Price = f.Bid.Add(f.Ask).DivInt(2)
		f.AskSize = t.Levels[0].AskSz
		f.BidSize = t.Levels[0].BidSz
	}
}

func onOptionDef(o *options.Option) {
	gOptionsByID[o.ID] = o
	gOptionsByOSI[o.OSI()] = o
}

func onOptionTick(t *databento.CMBP1) {
	if o := gOptionsByID[t.Header.InstrumentID]; o != nil {
		switch t.Action {
		case databento.ActionTrade:
			onOptionTrade(o, t)
		case databento.ActionAdd:
			if t.Header.TSEvent > o.TS {
				onOptionQuote(o, t)
			}
		default:
			panic("unexpected option tick action: " + t.Action.String())
		}
	}
}

func onOptionTrade(o *options.Option, t *databento.CMBP1) {
	// TODO(jart): log information about trades?
}

func onOptionQuote(o *options.Option, t *databento.CMBP1) {
	o.TS = t.Header.TSEvent
	mustRecomputeGreeks := false
	bid := t.Levels[0].BidPx
	if bid != databento.UndefPrice {
		price := decimal.Decimal(bid / 1000)
		if price.Cmp(o.Bid) != 0 {
			mustRecomputeGreeks = true
		}
		o.Bid = price
		o.BidSize = t.Levels[0].BidSz
		o.Got |= options.GotBid
	} else {
		o.Bid = decimal.Zero
		o.BidSize = 0
		o.Got &^= options.GotBid
	}
	ask := t.Levels[0].AskPx
	if ask != databento.UndefPrice {
		price := decimal.Decimal(ask / 1000)
		if price.Cmp(o.Ask) != 0 {
			mustRecomputeGreeks = true
		}
		o.Ask = price
		o.AskSize = t.Levels[0].AskSz
		o.Got |= options.GotAsk
	} else {
		o.Ask = decimal.Zero
		o.AskSize = 0
		o.Got &^= options.GotAsk
	}
	if gSPX.Add(o) {
		mustRecomputeGreeks = true
	}
	if mustRecomputeGreeks && gES != nil {
		o.ComputeGreeks(gSPX.Price, riskFreeRate(), gES.Price)
	}
}
