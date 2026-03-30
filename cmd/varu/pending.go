package main

func (t *Trader) addPendingOrder(order *Order) {
	t.PendingOrders[order] = true
	for _, leg := range order.Legs {
		t.PendingOrdersByOption[leg.Option] = append(t.PendingOrdersByOption[leg.Option], order)
	}
}

func (t *Trader) removePendingOrder(order *Order) {
	delete(t.PendingOrders, order)
	for _, leg := range order.Legs {
		orders := t.PendingOrdersByOption[leg.Option]
		for i, o := range orders {
			if o == order {
				orders = append(orders[:i], orders[i+1:]...)
				if len(orders) == 0 {
					delete(t.PendingOrdersByOption, leg.Option)
				} else {
					t.PendingOrdersByOption[leg.Option] = orders
				}
				break
			}
		}
	}
}
