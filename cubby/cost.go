package cubby

import (
	"dropbear/ds"
	"flag"
)

const defaultCostBasisMethod = ds.CostBasisMethodLIFO

var (
	flagHIFO = flag.Bool("hifo", false, "use HIFO cost basis calculation")
	flagLIFO = flag.Bool("lifo", false, "use LIFO cost basis calculation")
	flagFIFO = flag.Bool("fifo", false, "use FIFO cost basis calculation")
)

func GetCostBasisMethod() ds.CostBasisMethod {
	if *flagHIFO {
		return ds.CostBasisMethodHIFO
	}
	if *flagLIFO {
		return ds.CostBasisMethodLIFO
	}
	if *flagFIFO {
		return ds.CostBasisMethodFIFO
	}
	return defaultCostBasisMethod
}
