package databento

import "fmt"

type Venue uint16

const (
	VenueGLBX Venue = 1  // CME Globex
	VenueXNAS Venue = 2  // Nasdaq - All Markets
	VenueXBOS Venue = 3  // Nasdaq OMX BX
	VenueXPSX Venue = 4  // Nasdaq OMX PSX
	VenueBATS Venue = 5  // Cboe BZX U.S. Equities Exchange
	VenueBATY Venue = 6  // Cboe BYX U.S. Equities Exchange
	VenueEDGA Venue = 7  // Cboe EDGA U.S. Equities Exchange
	VenueEDGX Venue = 8  // Cboe EDGX U.S. Equities Exchange
	VenueXNYS Venue = 9  // New York Stock Exchange Inc.
	VenueXCIS Venue = 10 // NYSE National Inc.
	VenueXASE Venue = 11 // NYSE MKT LLC
	VenueARCX Venue = 12 // NYSE Arca
	VenueXCHI Venue = 13 // NYSE Texas Inc.
	VenueIEXG Venue = 14 // Investors Exchange
	VenueFINN Venue = 15 // FINRA/Nasdaq TRF Carteret
	VenueFINC Venue = 16 // FINRA/Nasdaq TRF Chicago
	VenueFINY Venue = 17 // FINRA/NYSE TRF
	VenueMEMX Venue = 18 // MEMX LLC Equities
	VenueEPRL Venue = 19 // MIAX Pearl Equities
	VenueAMXO Venue = 20 // NYSE American Options
	VenueXBOX Venue = 21 // BOX Options
	VenueXCBO Venue = 22 // Cboe Options
	VenueEMLD Venue = 23 // MIAX Emerald
	VenueEDGO Venue = 24 // Cboe EDGX Options
	VenueGMNI Venue = 25 // Nasdaq GEMX
	VenueXISX Venue = 26 // Nasdaq ISE
	VenueMCRY Venue = 27 // Nasdaq MRX
	VenueXMIO Venue = 28 // MIAX Options
	VenueARCO Venue = 29 // NYSE Arca Options
	VenueOPRA Venue = 30 // Options Price Reporting Authority
	VenueMPRL Venue = 31 // MIAX Pearl
	VenueXNDQ Venue = 32 // Nasdaq Options
	VenueXBXO Venue = 33 // Nasdaq BX Options
	VenueC2OX Venue = 34 // Cboe C2 Options
	VenueXPHL Venue = 35 // Nasdaq PHLX
	VenueBATO Venue = 36 // Cboe BZX Options
	VenueMXOP Venue = 37 // MEMX Options
	VenueIFEU Venue = 38 // ICE Europe Commodities
	VenueNDEX Venue = 39 // ICE Endex
	VenueDBEQ Venue = 40 // Databento US Equities - Consolidated
	VenueSPHR Venue = 41 // MIAX Sapphire
	VenueLTSE Venue = 42 // Long-Term Stock Exchange Inc.
	VenueXOFF Venue = 43 // Off-Exchange Transactions - Listed Instruments
	VenueASPN Venue = 44 // IntelligentCross ASPEN Intelligent Bid/Offer
	VenueASMT Venue = 45 // IntelligentCross ASPEN Maker/Taker
	VenueASPI Venue = 46 // IntelligentCross ASPEN Inverted
	VenueEQUS Venue = 47 // Databento US Equities - Consolidated
	VenueIFUS Venue = 48 // ICE Futures US
	VenueIFLL Venue = 49 // ICE Europe Financials
	VenueXEUR Venue = 50 // Eurex Exchange
	VenueXEEE Venue = 51 // European Energy Exchange
	VenueXCBF Venue = 52 // Cboe Futures Exchange
	VenueOCEA Venue = 53 // Blue Ocean ATS
)

func (v Venue) String() string {
	switch v {
	case VenueGLBX:
		return "GLBX"
	case VenueXNAS:
		return "XNAS"
	case VenueXBOS:
		return "XBOS"
	case VenueXPSX:
		return "XPSX"
	case VenueBATS:
		return "BATS"
	case VenueBATY:
		return "BATY"
	case VenueEDGA:
		return "EDGA"
	case VenueEDGX:
		return "EDGX"
	case VenueXNYS:
		return "XNYS"
	case VenueXCIS:
		return "XCIS"
	case VenueXASE:
		return "XASE"
	case VenueARCX:
		return "ARCX"
	case VenueXCHI:
		return "XCHI"
	case VenueIEXG:
		return "IEXG"
	case VenueFINN:
		return "FINN"
	case VenueFINC:
		return "FINC"
	case VenueFINY:
		return "FINY"
	case VenueMEMX:
		return "MEMX"
	case VenueEPRL:
		return "EPRL"
	case VenueAMXO:
		return "AMXO"
	case VenueXBOX:
		return "XBOX"
	case VenueXCBO:
		return "XCBO"
	case VenueEMLD:
		return "EMLD"
	case VenueEDGO:
		return "EDGO"
	case VenueGMNI:
		return "GMNI"
	case VenueXISX:
		return "XISX"
	case VenueMCRY:
		return "MCRY"
	case VenueXMIO:
		return "XMIO"
	case VenueARCO:
		return "ARCO"
	case VenueOPRA:
		return "OPRA"
	case VenueMPRL:
		return "MPRL"
	case VenueXNDQ:
		return "XNDQ"
	case VenueXBXO:
		return "XBXO"
	case VenueC2OX:
		return "C2OX"
	case VenueXPHL:
		return "XPHL"
	case VenueBATO:
		return "BATO"
	case VenueMXOP:
		return "MXOP"
	case VenueIFEU:
		return "IFEU"
	case VenueNDEX:
		return "NDEX"
	case VenueDBEQ:
		return "DBEQ"
	case VenueSPHR:
		return "SPHR"
	case VenueLTSE:
		return "LTSE"
	case VenueXOFF:
		return "XOFF"
	case VenueASPN:
		return "ASPN"
	case VenueASMT:
		return "ASMT"
	case VenueASPI:
		return "ASPI"
	case VenueEQUS:
		return "EQUS"
	case VenueIFUS:
		return "IFUS"
	case VenueIFLL:
		return "IFLL"
	case VenueXEUR:
		return "XEUR"
	case VenueXEEE:
		return "XEEE"
	case VenueXCBF:
		return "XCBF"
	case VenueOCEA:
		return "OCEA"
	default:
		return fmt.Sprintf("Venue(%d)", v)
	}
}

func (v Venue) GoString() string {
	switch v {
	case VenueGLBX:
		return "VenueGLBX"
	case VenueXNAS:
		return "VenueXNAS"
	case VenueXBOS:
		return "VenueXBOS"
	case VenueXPSX:
		return "VenueXPSX"
	case VenueBATS:
		return "VenueBATS"
	case VenueBATY:
		return "VenueBATY"
	case VenueEDGA:
		return "VenueEDGA"
	case VenueEDGX:
		return "VenueEDGX"
	case VenueXNYS:
		return "VenueXNYS"
	case VenueXCIS:
		return "VenueXCIS"
	case VenueXASE:
		return "VenueXASE"
	case VenueARCX:
		return "VenueARCX"
	case VenueXCHI:
		return "VenueXCHI"
	case VenueIEXG:
		return "VenueIEXG"
	case VenueFINN:
		return "VenueFINN"
	case VenueFINC:
		return "VenueFINC"
	case VenueFINY:
		return "VenueFINY"
	case VenueMEMX:
		return "VenueMEMX"
	case VenueEPRL:
		return "VenueEPRL"
	case VenueAMXO:
		return "VenueAMXO"
	case VenueXBOX:
		return "VenueXBOX"
	case VenueXCBO:
		return "VenueXCBO"
	case VenueEMLD:
		return "VenueEMLD"
	case VenueEDGO:
		return "VenueEDGO"
	case VenueGMNI:
		return "VenueGMNI"
	case VenueXISX:
		return "VenueXISX"
	case VenueMCRY:
		return "VenueMCRY"
	case VenueXMIO:
		return "VenueXMIO"
	case VenueARCO:
		return "VenueARCO"
	case VenueOPRA:
		return "VenueOPRA"
	case VenueMPRL:
		return "VenueMPRL"
	case VenueXNDQ:
		return "VenueXNDQ"
	case VenueXBXO:
		return "VenueXBXO"
	case VenueC2OX:
		return "VenueC2OX"
	case VenueXPHL:
		return "VenueXPHL"
	case VenueBATO:
		return "VenueBATO"
	case VenueMXOP:
		return "VenueMXOP"
	case VenueIFEU:
		return "VenueIFEU"
	case VenueNDEX:
		return "VenueNDEX"
	case VenueDBEQ:
		return "VenueDBEQ"
	case VenueSPHR:
		return "VenueSPHR"
	case VenueLTSE:
		return "VenueLTSE"
	case VenueXOFF:
		return "VenueXOFF"
	case VenueASPN:
		return "VenueASPN"
	case VenueASMT:
		return "VenueASMT"
	case VenueASPI:
		return "VenueASPI"
	case VenueEQUS:
		return "VenueEQUS"
	case VenueIFUS:
		return "VenueIFUS"
	case VenueIFLL:
		return "VenueIFLL"
	case VenueXEUR:
		return "VenueXEUR"
	case VenueXEEE:
		return "VenueXEEE"
	case VenueXCBF:
		return "VenueXCBF"
	case VenueOCEA:
		return "VenueOCEA"
	default:
		return fmt.Sprintf("Venue(%d)", v)
	}
}
