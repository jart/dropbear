package main

import "log"

func (t *Trader) addPendingOrder(order *Order) {
	log.Printf("adding order #%d to pending\n", order.ID)
	t.PendingOrders[order] = true
	t.PendingOrdersBySecurity[order.Security] = append(t.PendingOrdersBySecurity[order.Security], order)
}

func (t *Trader) removePendingOrder(order *Order) {
	log.Printf("removing order #%d from pending\n", order.ID)
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
