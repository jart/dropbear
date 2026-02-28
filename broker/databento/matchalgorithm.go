package databento

import "fmt"

type MatchAlgorithm byte

const (
	MatchAlgorithmUndefined                   MatchAlgorithm = ' '
	MatchAlgorithmFIFO                        MatchAlgorithm = 'F' // first-in-first-out matching
	MatchAlgorithmConfigurable                MatchAlgorithm = 'K' // configurable match algorithm
	MatchAlgorithmProRata                     MatchAlgorithm = 'C' // trade quantity is allocated to resting orders based on pro-rata percentage: resting order quantity divided by total quantity
	MatchAlgorithmFIFOLMM                     MatchAlgorithm = 'T' // like fifo but with lmm allocations prior to fifo allocations
	MatchAlgorithmThresholdProRata            MatchAlgorithm = 'O' // like pro-rata but includes configurable allocation to first order that improves market
	MatchAlgorithmFIFOTopLMM                  MatchAlgorithm = 'S' // like fifo lmm but includes configurable allocation to first order that improves market
	MatchAlgorithmThresholdProRataLMM         MatchAlgorithm = 'Q' // like threshold pro rata but includes special priority to lmms
	MatchAlgorithmEurodollarFutures           MatchAlgorithm = 'Y' // special variant used only by eurodollar futures on cme
	MatchAlgorithmTimeProRata                 MatchAlgorithm = 'P' // trade quantity is shared between all orders at best price; orders with highest time priority receive higher matched quantity
	MatchAlgorithmInstitutionalPrioritization MatchAlgorithm = 'V' // two-pass fifo algorithm; first pass fills institution group with whom aggressor is associated; second pass matches orders without institutional group association
)

func (ma MatchAlgorithm) String() string {
	switch ma {
	case 0:
		return "0"
	case MatchAlgorithmUndefined:
		return "MatchAlgorithmUndefined"
	case MatchAlgorithmFIFO:
		return "MatchAlgorithmFIFO"
	case MatchAlgorithmConfigurable:
		return "MatchAlgorithmConfigurable"
	case MatchAlgorithmProRata:
		return "MatchAlgorithmProRata"
	case MatchAlgorithmFIFOLMM:
		return "MatchAlgorithmFIFOLMM"
	case MatchAlgorithmThresholdProRata:
		return "MatchAlgorithmThresholdProRata"
	case MatchAlgorithmFIFOTopLMM:
		return "MatchAlgorithmFIFOTopLMM"
	case MatchAlgorithmThresholdProRataLMM:
		return "MatchAlgorithmThresholdProRataLMM"
	case MatchAlgorithmEurodollarFutures:
		return "MatchAlgorithmEurodollarFutures"
	case MatchAlgorithmTimeProRata:
		return "MatchAlgorithmTimeProRata"
	case MatchAlgorithmInstitutionalPrioritization:
		return "MatchAlgorithmInstitutionalPrioritization"
	default:
		return fmt.Sprintf("MatchAlgorithm(%d)", ma)
	}
}
