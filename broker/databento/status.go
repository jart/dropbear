package databento

import (
	"dropbear/clocky"
	"fmt"
	"strings"
)

// StatusMsg is an exchange status record (RType 0x12, 40 bytes).
// Unchanged across DBN versions v1/v2/v3.
type StatusMsg struct {
	Header                RecordHeader // 16 bytes
	TSRecv                clocky.Time  // capture-server-received timestamp
	Action                StatusAction // primary status action
	Reason                StatusReason // cause of halt or status change
	TradingEvent          TradingEvent // further information about the update
	IsTrading             TriState     // whether the instrument is trading
	IsQuoting             TriState     // whether the instrument is quoting
	IsShortSellRestricted TriState     // whether short selling is restricted
	_reserved             [7]byte
}

func (m *StatusMsg) GoString() string {
	var b strings.Builder
	b.WriteString("StatusMsg{\n")
	appendName(&b, "Header")
	appendField(&b, &m.Header)
	appendName(&b, "TSRecv")
	appendTimestamp(&b, m.TSRecv)
	appendName(&b, "Action")
	appendField(&b, m.Action)
	appendName(&b, "Reason")
	appendField(&b, m.Reason)
	appendName(&b, "TradingEvent")
	appendField(&b, m.TradingEvent)
	appendName(&b, "IsTrading")
	appendField(&b, m.IsTrading)
	appendName(&b, "IsQuoting")
	appendField(&b, m.IsQuoting)
	appendName(&b, "IsShortSellRestricted")
	appendField(&b, m.IsShortSellRestricted)
	b.WriteString("}")
	return b.String()
}

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

// StatusReason explains the cause of a halt or other change in action.
type StatusReason uint16

const (
	StatusReasonNone                        StatusReason = 0
	StatusReasonScheduled                   StatusReason = 1
	StatusReasonSurveillanceIntervention    StatusReason = 2
	StatusReasonMarketEvent                 StatusReason = 3
	StatusReasonInstrumentActivation        StatusReason = 4
	StatusReasonInstrumentExpiration        StatusReason = 5
	StatusReasonRecoveryInProcess           StatusReason = 6
	StatusReasonRegulatory                  StatusReason = 10
	StatusReasonAdministrative              StatusReason = 11
	StatusReasonNonCompliance               StatusReason = 12
	StatusReasonFilingsNotCurrent           StatusReason = 13
	StatusReasonSecTradingSuspension        StatusReason = 14
	StatusReasonNewIssue                    StatusReason = 15
	StatusReasonIssueAvailable              StatusReason = 16
	StatusReasonIssuesReviewed              StatusReason = 17
	StatusReasonFilingReqsSatisfied         StatusReason = 18
	StatusReasonNewsPending                 StatusReason = 30
	StatusReasonNewsReleased                StatusReason = 31
	StatusReasonNewsAndResumptionTimes      StatusReason = 32
	StatusReasonNewsNotForthcoming          StatusReason = 33
	StatusReasonOrderImbalance              StatusReason = 40
	StatusReasonLuldPause                   StatusReason = 50
	StatusReasonOperational                 StatusReason = 60
	StatusReasonAdditionalInfoRequested     StatusReason = 70
	StatusReasonMergerEffective             StatusReason = 80
	StatusReasonEtf                         StatusReason = 90
	StatusReasonCorporateAction             StatusReason = 100
	StatusReasonNewSecurityOffering         StatusReason = 110
	StatusReasonMarketWideHaltLevel1        StatusReason = 120
	StatusReasonMarketWideHaltLevel2        StatusReason = 121
	StatusReasonMarketWideHaltLevel3        StatusReason = 122
	StatusReasonMarketWideHaltCarryover     StatusReason = 123
	StatusReasonMarketWideHaltResumption    StatusReason = 124
	StatusReasonQuotationNotAvailable       StatusReason = 130
)

func (r StatusReason) String() string {
	switch r {
	case StatusReasonNone:
		return "None"
	case StatusReasonScheduled:
		return "Scheduled"
	case StatusReasonSurveillanceIntervention:
		return "SurveillanceIntervention"
	case StatusReasonMarketEvent:
		return "MarketEvent"
	case StatusReasonInstrumentActivation:
		return "InstrumentActivation"
	case StatusReasonInstrumentExpiration:
		return "InstrumentExpiration"
	case StatusReasonRecoveryInProcess:
		return "RecoveryInProcess"
	case StatusReasonRegulatory:
		return "Regulatory"
	case StatusReasonAdministrative:
		return "Administrative"
	case StatusReasonNonCompliance:
		return "NonCompliance"
	case StatusReasonFilingsNotCurrent:
		return "FilingsNotCurrent"
	case StatusReasonSecTradingSuspension:
		return "SecTradingSuspension"
	case StatusReasonNewIssue:
		return "NewIssue"
	case StatusReasonIssueAvailable:
		return "IssueAvailable"
	case StatusReasonIssuesReviewed:
		return "IssuesReviewed"
	case StatusReasonFilingReqsSatisfied:
		return "FilingReqsSatisfied"
	case StatusReasonNewsPending:
		return "NewsPending"
	case StatusReasonNewsReleased:
		return "NewsReleased"
	case StatusReasonNewsAndResumptionTimes:
		return "NewsAndResumptionTimes"
	case StatusReasonNewsNotForthcoming:
		return "NewsNotForthcoming"
	case StatusReasonOrderImbalance:
		return "OrderImbalance"
	case StatusReasonLuldPause:
		return "LuldPause"
	case StatusReasonOperational:
		return "Operational"
	case StatusReasonAdditionalInfoRequested:
		return "AdditionalInfoRequested"
	case StatusReasonMergerEffective:
		return "MergerEffective"
	case StatusReasonEtf:
		return "Etf"
	case StatusReasonCorporateAction:
		return "CorporateAction"
	case StatusReasonNewSecurityOffering:
		return "NewSecurityOffering"
	case StatusReasonMarketWideHaltLevel1:
		return "MarketWideHaltLevel1"
	case StatusReasonMarketWideHaltLevel2:
		return "MarketWideHaltLevel2"
	case StatusReasonMarketWideHaltLevel3:
		return "MarketWideHaltLevel3"
	case StatusReasonMarketWideHaltCarryover:
		return "MarketWideHaltCarryover"
	case StatusReasonMarketWideHaltResumption:
		return "MarketWideHaltResumption"
	case StatusReasonQuotationNotAvailable:
		return "QuotationNotAvailable"
	default:
		return fmt.Sprintf("StatusReason(%d)", r)
	}
}

// TradingEvent provides further information about a status update.
type TradingEvent uint16

const (
	TradingEventNone                TradingEvent = 0
	TradingEventNoCancel            TradingEvent = 1
	TradingEventChangeTradingSession TradingEvent = 2
	TradingEventImpliedMatchingOn   TradingEvent = 3
	TradingEventImpliedMatchingOff  TradingEvent = 4
)

func (e TradingEvent) String() string {
	switch e {
	case TradingEventNone:
		return "None"
	case TradingEventNoCancel:
		return "NoCancel"
	case TradingEventChangeTradingSession:
		return "ChangeTradingSession"
	case TradingEventImpliedMatchingOn:
		return "ImpliedMatchingOn"
	case TradingEventImpliedMatchingOff:
		return "ImpliedMatchingOff"
	default:
		return fmt.Sprintf("TradingEvent(%d)", e)
	}
}

// TriState represents unknown, true, or false.
type TriState byte

const (
	TriStateNotAvailable TriState = '~'
	TriStateNo           TriState = 'N'
	TriStateYes          TriState = 'Y'
)

func (t TriState) String() string {
	switch t {
	case TriStateNotAvailable:
		return "N/A"
	case TriStateNo:
		return "No"
	case TriStateYes:
		return "Yes"
	default:
		return fmt.Sprintf("TriState(%d)", t)
	}
}
