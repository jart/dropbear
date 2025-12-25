package ds

// CostBasisMethod specifies FIFO, LIFO, or HIFO lot consumption order.
type CostBasisMethod int

const (
	CostBasisMethodHIFO CostBasisMethod = iota // highest in, first out
	CostBasisMethodLIFO                        // last in, first out
	CostBasisMethodFIFO                        // first in, first out
)

// String returns the string representation of the CostBasisMethod.
func (cbm CostBasisMethod) String() string {
	switch cbm {
	case CostBasisMethodHIFO:
		return "COST_BASIS_METHOD_HIFO"
	case CostBasisMethodLIFO:
		return "COST_BASIS_METHOD_LIFO"
	case CostBasisMethodFIFO:
		return "COST_BASIS_METHOD_FIFO"
	default:
		panic("unknown CostBasisMethod")
	}
}
