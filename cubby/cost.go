package cubby

import (
	"dropbear/ds"
	"flag"
)

// LIFO is the default cost basis method, because:
//  1. It enables you to market make on top of long term holdings,
//     so most of your inventory stands a chance of being taxed in
//     the US at lower capital gains rates. FIFO and HIFO will churn
//     through all your lots, eventually making them all short-term.
//  2. It enables higher trading volume, since your trading algorithm
//     only needs to be profitable with respect to recently opened
//     positions, rather than what it decided a long time ago. This
//     helps solve the ratchet effect with HIFO, where any market
//     downturn will force all sales to become unprofitable.
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
