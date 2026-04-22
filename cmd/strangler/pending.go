package main

func (t *Trader) addPendingOrder(order *Order) {
	t.PendingOrders[order] = true
	t.PendingOrdersBySecurity[order.Security] = append(t.PendingOrdersBySecurity[order.Security], order)
}

func (t *Trader) removePendingOrder(order *Order) {
	for _, clientOrderID := range order.clientOrderIDs {
		delete(t.OrdersByClientOrderID, clientOrderID)
	}
	delete(t.PendingOrders, order)
	orders := t.PendingOrdersBySecurity[order.Security]
	for i, o := range orders {
		if o == order {
			orders = append(orders[:i], orders[i+1:]...)
			if len(orders) == 0 {
				delete(t.PendingOrdersBySecurity, order.Security)
			} else {
				t.PendingOrdersBySecurity[order.Security] = orders
			}
			break
		}
	}
}
