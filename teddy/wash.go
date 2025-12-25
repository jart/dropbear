package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
)

// self-trading is insane because it appears ot be one of the most legitimately dangerous
// practices with the potential to gigafry your brokerage account but is exclusively done
// by literal turbonormies who unironically want to "improve user engagement metrics" and
// basically get oneshotted by regulators.
func (p *Pair) checkSelfTrade(side ds.Side, limitPrice decimal.Decimal) error {
	p.Lock.RLock()
	defer p.Lock.RUnlock()
	for it := p.openOrders.Iterator(); it.Next(); {
		order := it.Value()
		if order.Side == side.Flip() && limitPrice.Mul(decimal.Decimal(side)).Cmp(order.LimitPrice) >= 0 {
			loggy.Fatalf("self-trade detected: your %s limitPrice %s crosses your other order's limitPrice %s", side, limitPrice, order.LimitPrice)
			return ds.ErrSelfTrade
		}
	}
	return nil
}
