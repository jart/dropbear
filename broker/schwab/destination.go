package schwab

type OrderDestination string

const (
	OrderDestinationDFIN = OrderDestination("DFIN") // Dash Financial (ION Group) and IMC Financial Markets
	OrderDestinationCDRG = OrderDestination("CDRG") // Citadel Derivatives Group
	OrderDestinationJNST = OrderDestination("JNST") // Jane Street
	OrderDestinationSIGQ = OrderDestination("SIGQ") // Susquehanna (SIG)
	OrderDestinationWEXM = OrderDestination("WEXM") // Wolverine Execution Services
	OrderDestinationSOHO = OrderDestination("SOHO") // Two Sigma Securities
	OrderDestinationNSDQ = OrderDestination("NSDQ") // NASDAQ exchange direct
	OrderDestinationNITE = OrderDestination("NITE") // Virtu Financial (formerly Knight Capital, "KCG NITE")
	OrderDestinationETMM = OrderDestination("ETMM") // E*Trade Market Making (Morgan Stanley)
)
