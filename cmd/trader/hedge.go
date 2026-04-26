package main

import (
	"dropbear/broker/alpaca"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"log"
)

// manageHedge buy/sells index when portfolio shorts/longs are unbalanced.
//
// here's our threat vector. this strategy usually spots long and short
// opportunities in equal proportion. so under normal conditions, our net
// exposure should be around zero, meaning whichever way the broader market
// moves, it's unlikely to impact our account equity. however, we place an
// enormous amount of resting orders on the book that are sitting around waiting
// to be filled. we could have hundreds of buy and sell orders that could
// potentially be filled at any moment. now here's the issue. if the broader
// market goes nuts, we could end up in a situation where hundreds of buy orders
// suddenly get filled, while hundreds of sell orders get ignored, leaving us
// with magnificent long exposure just at the moment the market crashes.
//
// to solve that, we call this subroutine every second to check the balance of
// our entire portfolio. if we have too many long positions and not enough short
// positions, then we aggressively sell as many shares of the russell 2000 index
// fund as it takes to bring us back to delta neutrality. since we're willing to
// trade just about any stock listed on the major exchanges, no matter how awful
// or insignificent it is, the hope is that our positions are diversified enough
// that our losses will be equally correlated with our gains shorting the small
// cap index, and since iwm is a very liquid etf, we should be able to get in
// and out without without too much slippage.
//
// our strategy for executing hedge trades is a little different than our normal
// strategy. during the day, we take liquidity and almost any price, in the hope
// that there'll be enough liquidity that the market gives us a good price. when
// it's extended or overnight hours, that would obviously be an epic disaster so
// we sneakily try to hedge slowly by morning by making the index fund. if it is
// filled, then our order is canceled and reposted every -patience=1h which will
// hopefully avoid the adverserial and illiquid nature of the market at night.
func manageHedge() {
	if *flagHedge == "" || flagRisk.IsZero() {
		return // hedging disabled
	}
	st := gSymbols[*flagHedge]
	if st == nil || st.quote == nil {
		return // can't hedge without a quote
	}
	if st.buyClientOrderID != "" || st.sellClientOrderID != "" {
		return // already in the process of hedging
	}
	exposure := getMarketExposure()
	if exposure.Cmp(*flagRisk) > 0 {
		reduceLongExposure(st, exposure)
	} else if exposure.Cmp(flagRisk.Neg()) < 0 {
		reduceShortExposure(st, exposure.Neg())
	}
}

func reduceLongExposure(st *State, longExposure decimal.Decimal) {
	// we're net long and need to hedge by selling
	var price decimal.Decimal
	session := cboe.GetSession(clocky.Now())
	switch session {
	case cboe.SessionDay:
		price = st.quote.BidPrice.Sub(decimal.One) // take by day
	case cboe.SessionOvernight, cboe.SessionExtended:
		if st.quote.Indicative() || st.quote.DangerousAsk() {
			return // quote isn't reliable
		}
		price = st.quote.AskPrice // make by night
	default:
		return // market is closed
	}
	qty := longExposure.Div(price)
	// avoid overtrading
	lot := cboe.LotSize(price)
	qty = qty.QuantizeTruncate(lot)
	// flatten before reversing polarity
	if qty.Cmp(st.position) > 0 {
		qty = st.position
	}
	if qty.IsPositive() {
		dest := alpaca.OrderDestinationNone
		log.Printf("hedging long exposure of %s by selling %s shares of %s at %s",
			longExposure, qty, st.symbol, price)
		LimitOrder(st, ds.SideSell, qty, price, dest, session)
	}
}

func reduceShortExposure(st *State, shortExposure decimal.Decimal) {
	// we're net short and need to hedge by buying
	var price decimal.Decimal
	session := cboe.GetSession(clocky.Now())
	switch session {
	case cboe.SessionDay:
		price = st.quote.AskPrice.Add(decimal.One) // take by day
	case cboe.SessionOvernight, cboe.SessionExtended:
		if st.quote.Indicative() || st.quote.DangerousBid() {
			return // quote isn't reliable
		}
		price = st.quote.BidPrice // make by night
	default:
		return // market is closed
	}
	qty := shortExposure.Div(price)
	// avoid overtrading
	lot := cboe.LotSize(price)
	qty = qty.QuantizeTruncate(lot)
	// flatten before reversing polarity
	if qty.Cmp(st.position.Neg()) > 0 {
		qty = st.position.Neg()
	}
	if qty.IsPositive() {
		dest := alpaca.OrderDestinationNone
		log.Printf("hedging short exposure of %s by buying %s shares of %s at %s",
			shortExposure, qty, st.symbol, price)
		LimitOrder(st, ds.SideBuy, qty, price, dest, session)
	}
}

// getMarketExposure computes how much it'd cost to liquidate portfolio at market price.
// here we take a more pessimistic view than the normal the mark-to-market price calculation.
func getMarketExposure() decimal.Decimal {
	exposure := decimal.Zero
	for _, st := range gSymbols {
		if st.quote == nil {
			continue // can't compute exposure without quote
		}
		if st.position.IsPositive() {
			notional := st.quote.BidPrice.Mul(st.position)
			exposure = exposure.Add(notional)
		} else if st.position.IsNegative() {
			notional := st.quote.AskPrice.Mul(st.position.Neg())
			exposure = exposure.Sub(notional)
		}
	}
	return exposure
}
