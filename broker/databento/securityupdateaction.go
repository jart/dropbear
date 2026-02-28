package databento

import "fmt"

type SecurityUpdateAction byte

const (
	SecurityUpdateActionAdd    SecurityUpdateAction = 'A'
	SecurityUpdateActionModify SecurityUpdateAction = 'M'
	SecurityUpdateActionDelete SecurityUpdateAction = 'D'
)

func (sua SecurityUpdateAction) String() string {
	switch sua {
	case 0:
		return "0"
	case SecurityUpdateActionAdd:
		return "SecurityUpdateActionAdd"
	case SecurityUpdateActionModify:
		return "SecurityUpdateActionModify"
	case SecurityUpdateActionDelete:
		return "SecurityUpdateActionDelete"
	default:
		return fmt.Sprintf("SecurityUpdateAction(%d)", sua)
	}
}
