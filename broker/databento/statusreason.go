package databento

import (
	"fmt"
)

// StatusReason explains the cause of a halt or other change in action.
type StatusReason uint16

const (
	StatusReasonNone                     StatusReason = 0
	StatusReasonScheduled                StatusReason = 1
	StatusReasonSurveillanceIntervention StatusReason = 2
	StatusReasonMarketEvent              StatusReason = 3
	StatusReasonInstrumentActivation     StatusReason = 4
	StatusReasonInstrumentExpiration     StatusReason = 5
	StatusReasonRecoveryInProcess        StatusReason = 6
	StatusReasonRegulatory               StatusReason = 10
	StatusReasonAdministrative           StatusReason = 11
	StatusReasonNonCompliance            StatusReason = 12
	StatusReasonFilingsNotCurrent        StatusReason = 13
	StatusReasonSecTradingSuspension     StatusReason = 14
	StatusReasonNewIssue                 StatusReason = 15
	StatusReasonIssueAvailable           StatusReason = 16
	StatusReasonIssuesReviewed           StatusReason = 17
	StatusReasonFilingReqsSatisfied      StatusReason = 18
	StatusReasonNewsPending              StatusReason = 30
	StatusReasonNewsReleased             StatusReason = 31
	StatusReasonNewsAndResumptionTimes   StatusReason = 32
	StatusReasonNewsNotForthcoming       StatusReason = 33
	StatusReasonOrderImbalance           StatusReason = 40
	StatusReasonLuldPause                StatusReason = 50
	StatusReasonOperational              StatusReason = 60
	StatusReasonAdditionalInfoRequested  StatusReason = 70
	StatusReasonMergerEffective          StatusReason = 80
	StatusReasonEtf                      StatusReason = 90
	StatusReasonCorporateAction          StatusReason = 100
	StatusReasonNewSecurityOffering      StatusReason = 110
	StatusReasonMarketWideHaltLevel1     StatusReason = 120
	StatusReasonMarketWideHaltLevel2     StatusReason = 121
	StatusReasonMarketWideHaltLevel3     StatusReason = 122
	StatusReasonMarketWideHaltCarryover  StatusReason = 123
	StatusReasonMarketWideHaltResumption StatusReason = 124
	StatusReasonQuotationNotAvailable    StatusReason = 130
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
