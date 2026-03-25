package databento

import (
	"fmt"
)

// StatusAction is the primary enum for the type of StatusMsg update.
type StatusAction uint16

const (
	StatusActionNone                   StatusAction = 0
	StatusActionPreOpen                StatusAction = 1
	StatusActionPreCross               StatusAction = 2
	StatusActionQuoting                StatusAction = 3
	StatusActionCross                  StatusAction = 4
	StatusActionRotation               StatusAction = 5
	StatusActionNewPriceIndication     StatusAction = 6
	StatusActionTrading                StatusAction = 7
	StatusActionHalt                   StatusAction = 8
	StatusActionPause                  StatusAction = 9
	StatusActionSuspend                StatusAction = 10
	StatusActionPreClose               StatusAction = 11
	StatusActionClose                  StatusAction = 12
	StatusActionPostClose              StatusAction = 13
	StatusActionSsrChange              StatusAction = 14
	StatusActionNotAvailableForTrading StatusAction = 15
)

func (a StatusAction) String() string {
	switch a {
	case StatusActionNone:
		return "None"
	case StatusActionPreOpen:
		return "PreOpen"
	case StatusActionPreCross:
		return "PreCross"
	case StatusActionQuoting:
		return "Quoting"
	case StatusActionCross:
		return "Cross"
	case StatusActionRotation:
		return "Rotation"
	case StatusActionNewPriceIndication:
		return "NewPriceIndication"
	case StatusActionTrading:
		return "Trading"
	case StatusActionHalt:
		return "Halt"
	case StatusActionPause:
		return "Pause"
	case StatusActionSuspend:
		return "Suspend"
	case StatusActionPreClose:
		return "PreClose"
	case StatusActionClose:
		return "Close"
	case StatusActionPostClose:
		return "PostClose"
	case StatusActionSsrChange:
		return "SsrChange"
	case StatusActionNotAvailableForTrading:
		return "NotAvailableForTrading"
	default:
		return fmt.Sprintf("StatusAction(%d)", a)
	}
}
