package cubby

import "dropbear/clocky"

type liveTrader struct {
}

func newLiveTrader() *liveTrader {
	return &liveTrader{}
}

func (lt *liveTrader) Close() error {
	return Client.CancelAllOrders()
}

func (lt *liveTrader) Run() {

	// enable warmup mode
	IsWarmingUp = true

	// fetch last 1000 bars for all Equities
	// the last bar should be the most recent closed minute
	Client.GetBarsForSymbols(...)

	// now do something similar to backtest.go where we replay all the minute bars in order
	// be prepared for missing bars for some equities or gaps or whatever

	// disable warmup mode
	IsWarmingUp = false

	// now run the live trading loop
	// call Client.GetBarsForSymbols(...) every minute to get the latest bar for each symbol
	// try to ensure the call goes out as soon as possible after the minute closes
	// be prepared for an equity.OnTrade() callback taking potentially minutes
	// you can probably just use the last minute we know we processed bars to set the range
}
