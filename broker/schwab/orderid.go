package schwab

import (
	"fmt"
	"strconv"
)

// OrderID is an int64 that unmarshals from both JSON strings ("123") and numbers (123).
type OrderID int64

func (id OrderID) String() string {
	return strconv.FormatInt(int64(id), 10)
}

func (id OrderID) MarshalJSON() ([]byte, error) {
	// schwab websocket API sends order ids as strings
	// however schwab's ordering api wants order ids as numbers
	// so we encode as an integer since websocket is receive only
	return []byte(strconv.FormatInt(int64(id), 10)), nil
}

func (id *OrderID) UnmarshalJSON(data []byte) error {
	if data[0] == '"' {
		data = data[1 : len(data)-1]
	}
	i := 0
	var x int64
	for i < len(data) {
		if data[i] >= '0' && data[i] <= '9' {
			x *= 10
			x += int64(data[i] - '0')
			i++
		} else {
			return fmt.Errorf("schwab: invalid order id: %s", data)
		}
	}
	*id = OrderID(x)
	return nil
}
