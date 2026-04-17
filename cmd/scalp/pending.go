package main

func (t *Trader) addPendingOrder(order *Order) {
	t.PendingOrders[order] = true
	for _, leg := range order.Legs {
		t.PendingOrdersBySecurity[leg.Security] = append(t.PendingOrdersBySecurity[leg.Security], order)
	}
}

func (t *Trader) removePendingOrder(order *Order) {
	delete(t.PendingOrders, order)
	for _, leg := range order.Legs {
		orders := t.PendingOrdersBySecurity[leg.Security]
		for i, o := range orders {
			if o == order {
				orders = append(orders[:i], orders[i+1:]...)
				if len(orders) == 0 {
					delete(t.PendingOrdersBySecurity, leg.Security)
				} else {
					t.PendingOrdersBySecurity[leg.Security] = orders
				}
				break
			}
		}
	}
}
