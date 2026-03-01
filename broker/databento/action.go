// Action enum for order event or order book operation.
// Source: ~/vendor/databento-cpp/include/databento/enums.hpp lines 140-157
package databento

import "fmt"

type Action byte

const (
	ActionModify Action = 'M' // existing order modified: price and/or size
	ActionTrade  Action = 'T' // aggressing order traded, does not affect book
	ActionFill   Action = 'F' // existing order filled, does not affect book
	ActionCancel Action = 'C' // order fully or partially cancelled
	ActionAdd    Action = 'A' // new order added to the book
	ActionClear  Action = 'R' // reset book; clear all orders for instrument
	ActionNone   Action = 'N' // no effect on book, may carry flags or other info
)

func (a Action) String() string {
	switch a {
	case 0:
		return "0"
	case ActionModify:
		return "ActionModify"
	case ActionTrade:
		return "ActionTrade"
	case ActionFill:
		return "ActionFill"
	case ActionCancel:
		return "ActionCancel"
	case ActionAdd:
		return "ActionAdd"
	case ActionClear:
		return "ActionClear"
	case ActionNone:
		return "ActionNone"
	default:
		return fmt.Sprintf("Action(%d)", a)
	}
}

func (a Action) GoString() string {
	return a.String()
}
