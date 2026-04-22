package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/loggy"
	"flag"
	"fmt"
)

var (
	flagFeed     = flag.String("feed", "sip", "websocket url or name (sip, iex, boats, overnight, delayed, crypto)")
	flagTrades   = flag.Bool("trades", false, "subscribe to trade updates")
	flagQuotes   = flag.Bool("quotes", false, "subscribe to quote updates")
	flagStatuses = flag.Bool("statuses", false, "subscribe to status updates")
	flagLULDs    = flag.Bool("lulds", false, "subscribe to limit-up-limit-down updates (not available in boats)")
	flagNASDAQ   = flag.Bool("nasdaq", false, "subscribe to NASDAQ updates")
	flagNYSE     = flag.Bool("nyse", false, "subscribe to NYSE updates")
	flagARCA     = flag.Bool("arca", false, "subscribe to ARCA updates")
)

var kFeedURLs = map[string]string{
	"sip":       alpaca.SIPWSURL,
	"iex":       alpaca.IEXWSURL,
	"boats":     alpaca.BOATSWSURL,
	"overnight": alpaca.OvernightSWSURL,
	"delayed":   alpaca.DelayedSIPWSURL,
	"crypto":    alpaca.CryptoWSURL,
}

func main() {

	loggy.Init()
	flag.Parse()
	symbols := flag.Args()
	if !*flagTrades && !*flagQuotes && !*flagStatuses && !*flagLULDs {
		fmt.Println("At least one of -trades, -quotes, -statuses, or -lulds must be set")
		return
	}

	// figure out websocket url
	url := *flagFeed
	if u, ok := kFeedURLs[url]; ok {
		url = u
	}

	// figure out symbols we want
	if len(symbols) == 0 {
		symbols = []string{"*"}
	}

	// connect to websocket server
	req := &alpaca.StockUpdatesRequest{Action: "subscribe"}
	if *flagTrades {
		req.Trades = symbols
	}
	if *flagQuotes {
		req.Quotes = symbols
	}
	if *flagStatuses {
		req.Statuses = symbols
	}
	if *flagLULDs {
		req.LULDs = symbols
	}
	messages := alpaca.StockUpdates(url, req)

	for msg := range messages {
		switch msg.Type {
		case sip.MessageTypeQuote:
			quote := msg.Quote()
			if *flagNASDAQ || *flagNYSE || *flagARCA {
				ok := false
				switch quote.BidExchange {
				case sip.ExchangeNASDAQ:
					if *flagNASDAQ {
						ok = true
					}
				case sip.ExchangeNYSE:
					if *flagNYSE {
						ok = true
					}
				case sip.ExchangeARCA:
					if *flagARCA {
						ok = true
					}
				}
				switch quote.AskExchange {
				case sip.ExchangeNASDAQ:
					if *flagNASDAQ {
						ok = true
					}
				case sip.ExchangeNYSE:
					if *flagNYSE {
						ok = true
					}
				case sip.ExchangeARCA:
					if *flagARCA {
						ok = true
					}
				}
				if !ok {
					continue
				}
			}
			fmt.Printf("%8s quote %c bid %8s x %5d @ %c | ask %8s x %5d @ %c | %s\n",
				quote.Symbol, quote.Tape,
				quote.BidPrice, quote.BidSize, quote.BidExchange,
				quote.AskPrice, quote.AskSize, quote.AskExchange,
				quote.Conditions.String())
		case sip.MessageTypeTrade:
			trade := msg.Trade()
			if *flagNASDAQ || *flagNYSE || *flagARCA {
				ok := false
				switch trade.Exchange {
				case sip.ExchangeNASDAQ:
					if *flagNASDAQ {
						ok = true
					}
				case sip.ExchangeNYSE:
					if *flagNYSE {
						ok = true
					}
				case sip.ExchangeARCA:
					if *flagARCA {
						ok = true
					}
				}
				if !ok {
					continue
				}
			}
			fmt.Printf("%8s trade %c price %8s x %5d @ %c | %s\n",
				trade.Symbol, trade.Tape,
				trade.Price, trade.Size, trade.Exchange,
				trade.Conditions.String())
		case sip.MessageTypeLULD:
			luld := msg.LULD()
			fmt.Printf("%8s luld %c price %8s | %s\n",
				luld.Symbol, luld.Tape,
				luld.LowerLimit,
				luld.UpperLimit)
		case sip.MessageTypeStatus:
			status := msg.Status()
			fmt.Printf("%8s status %c code %s reason %s message %s reasonmsg %s\n",
				status.Symbol, status.Tape,
				status.Code, status.Reason,
				status.Message, status.ReasonMsg)
		}
	}
}
