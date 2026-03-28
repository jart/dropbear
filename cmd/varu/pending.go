package main

import (
	"dropbear/ds/options"
)

var (
	gPendingOrders         = map[*Order]bool{}
	gPendingOrdersByOption = map[*options.Option][]*Order{}
)

func addPendingOrder(order *Order) {
	gPendingOrders[order] = true
	for _, leg := range order.Legs {
		gPendingOrdersByOption[leg.Option] = append(gPendingOrdersByOption[leg.Option], order)
	}
}

func removePendingOrder(order *Order) {
	delete(gPendingOrders, order)
	for _, leg := range order.Legs {
		orders := gPendingOrdersByOption[leg.Option]
		for i, o := range orders {
			if o == order {
				gPendingOrdersByOption[leg.Option] = append(orders[:i], orders[i+1:]...)
				break
			}
		}
	}
}
