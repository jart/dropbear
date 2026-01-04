package alpaca

import "fmt"

type AccountStatus uint8

const (
	AccountStatusOnboarding       AccountStatus = iota // account is onboarding
	AccountStatusSubmissionFailed                      // account application submission failed
	AccountStatusSubmitted                             // account application has been submitted for review
	AccountStatusAccountUpdated                        // account information is being updated
	AccountStatusApprovalPending                       // final account approval is pending
	AccountStatusActive                                // account is active for trading
	AccountStatusRejected                              // account application has been rejected
)

func ParseAccountStatus(s string) (AccountStatus, error) {
	switch s {
	case "ONBOARDING":
		return AccountStatusOnboarding, nil
	case "SUBMISSION_FAILED":
		return AccountStatusSubmissionFailed, nil
	case "SUBMITTED":
		return AccountStatusSubmitted, nil
	case "ACCOUNT_UPDATED":
		return AccountStatusAccountUpdated, nil
	case "APPROVAL_PENDING":
		return AccountStatusApprovalPending, nil
	case "ACTIVE":
		return AccountStatusActive, nil
	case "REJECTED":
		return AccountStatusRejected, nil
	default:
		return 0, fmt.Errorf("unknown account status: %s", s)
	}
}

func (as AccountStatus) String() string {
	switch as {
	case AccountStatusOnboarding:
		return "ONBOARDING"
	case AccountStatusSubmissionFailed:
		return "SUBMISSION_FAILED"
	case AccountStatusSubmitted:
		return "SUBMITTED"
	case AccountStatusAccountUpdated:
		return "ACCOUNT_UPDATED"
	case AccountStatusApprovalPending:
		return "APPROVAL_PENDING"
	case AccountStatusActive:
		return "ACTIVE"
	case AccountStatusRejected:
		return "REJECTED"
	default:
		panic("unknown account status")
	}
}

func (as AccountStatus) GoString() string {
	switch as {
	case AccountStatusOnboarding:
		return "AccountStatusOnboarding"
	case AccountStatusSubmissionFailed:
		return "AccountStatusSubmissionFailed"
	case AccountStatusSubmitted:
		return "AccountStatusSubmitted"
	case AccountStatusAccountUpdated:
		return "AccountStatusAccountUpdated"
	case AccountStatusApprovalPending:
		return "AccountStatusApprovalPending"
	case AccountStatusActive:
		return "AccountStatusActive"
	case AccountStatusRejected:
		return "AccountStatusRejected"
	default:
		panic("unknown account status")
	}
}

func (as AccountStatus) MarshalJSON() ([]byte, error) {
	return []byte(`"` + as.String() + `"`), nil
}

func (as *AccountStatus) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid account status: %s", data)
	}
	v, err := ParseAccountStatus(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*as = v
	return nil
}
