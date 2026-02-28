package databento

type SecurityUpdateAction byte

const (
	SecurityUpdateActionAdd    SecurityUpdateAction = 'A'
	SecurityUpdateActionModify SecurityUpdateAction = 'M'
	SecurityUpdateActionDelete SecurityUpdateAction = 'D'
)

func (sua SecurityUpdateAction) String() string {
	switch sua {
	case SecurityUpdateActionAdd:
		return "add"
	case SecurityUpdateActionModify:
		return "modify"
	case SecurityUpdateActionDelete:
		return "delete"
	default:
		return string(sua)
	}
}

func (sua SecurityUpdateAction) GoString() string {
	switch sua {
	case SecurityUpdateActionAdd:
		return "SecurityUpdateActionAdd"
	case SecurityUpdateActionModify:
		return "SecurityUpdateActionModify"
	case SecurityUpdateActionDelete:
		return "SecurityUpdateActionDelete"
	default:
		return string(sua)
	}
}
