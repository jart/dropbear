package databento

import "strconv"

type Publisher uint16

const (
	PublisherArcxPillar      = 43  // NYSE Arca Integrated
	PublisherBatsPitch       = 5   // Cboe BZX Depth
	PublisherBatyPitch       = 6   // Cboe BYX Depth
	PublisherDbeqBasic       = 59  // Databento US Equities Basic - Consolidated
	PublisherDbeqBasicEprl   = 42  // DBEQ Basic - MIAX Pearl
	PublisherDbeqBasicIexg   = 41  // DBEQ Basic - IEX
	PublisherDbeqBasicXchi   = 39  // DBEQ Basic - NYSE Texas
	PublisherDbeqBasicXcis   = 40  // DBEQ Basic - NYSE National
	PublisherEdgaPitch       = 7   // Cboe EDGA Depth
	PublisherEdgxPitch       = 8   // Cboe EDGX Depth
	PublisherEprlDom         = 16  // MIAX Pearl Depth
	PublisherEqusAll         = 94  // Databento US Equities (All Feeds) - Consolidated
	PublisherEqusAllArcx     = 79  // Databento US Equities (All Feeds) - NYSE Arca
	PublisherEqusAllBats     = 71  // Databento US Equities (All Feeds) - Cboe BZX
	PublisherEqusAllBaty     = 72  // Databento US Equities (All Feeds) - Cboe BYX
	PublisherEqusAllEdga     = 73  // Databento US Equities (All Feeds) - Cboe EDGA
	PublisherEqusAllEdgx     = 74  // Databento US Equities (All Feeds) - Cboe EDGX
	PublisherEqusAllEprl     = 65  // Databento US Equities (All Feeds) - MIAX Pearl
	PublisherEqusAllFinc     = 70  // Databento US Equities (All Feeds) - FINRA/Nasdaq TRF Chicago
	PublisherEqusAllFinn     = 68  // Databento US Equities (All Feeds) - FINRA/Nasdaq TRF Carteret
	PublisherEqusAllFiny     = 69  // Databento US Equities (All Feeds) - FINRA/NYSE TRF
	PublisherEqusAllIexg     = 64  // Databento US Equities (All Feeds) - IEX
	PublisherEqusAllLtse     = 80  // Databento US Equities (All Feeds) - Long-Term Stock Exchange
	PublisherEqusAllMemx     = 77  // Databento US Equities (All Feeds) - MEMX
	PublisherEqusAllXase     = 78  // Databento US Equities (All Feeds) - NYSE American
	PublisherEqusAllXbos     = 75  // Databento US Equities (All Feeds) - Nasdaq BX
	PublisherEqusAllXchi     = 62  // Databento US Equities (All Feeds) - NYSE Texas
	PublisherEqusAllXcis     = 63  // Databento US Equities (All Feeds) - NYSE National
	PublisherEqusAllXnas     = 66  // Databento US Equities (All Feeds) - Nasdaq
	PublisherEqusAllXnys     = 67  // Databento US Equities (All Feeds) - NYSE
	PublisherEqusAllXpsx     = 76  // Databento US Equities (All Feeds) - Nasdaq PSX
	PublisherEqusMini        = 95  // Databento US Equities Mini (EQUS.MINI)
	PublisherEqusPlusEprl    = 51  // Databento US Equities Plus - MIAX Pearl
	PublisherEqusPlusEqus    = 60  // EQUS Plus - Consolidated
	PublisherEqusPlusFinc    = 56  // Databento US Equities Plus - FINRA/Nasdaq TRF Chicago
	PublisherEqusPlusFinn    = 54  // Databento US Equities Plus - FINRA/Nasdaq TRF Carteret
	PublisherEqusPlusFiny    = 55  // Databento US Equities Plus - FINRA/NYSE TRF
	PublisherEqusPlusIexg    = 50  // Databento US Equities Plus - IEX
	PublisherEqusPlusXchi    = 48  // Databento US Equities Plus - NYSE Texas
	PublisherEqusPlusXcis    = 49  // Databento US Equities Plus - NYSE National
	PublisherEqusPlusXnas    = 52  // Databento US Equities Plus - Nasdaq
	PublisherEqusPlusXnys    = 53  // Databento US Equities Plus - NYSE
	PublisherEqusSummaryEqus = 90  // Databento Equities Summary
	PublisherGlbxMdp3        = 1   // CME Globex MDP 3.0
	PublisherIexgTopsIexg    = 38  // IEX TOPS
	PublisherIfeuImpact      = 57  // ICE Europe Commodities
	PublisherIfeuImpactXoff  = 84  // ICE Europe - Off-Market Trades
	PublisherIfllImpact      = 99  // ICE Europe Financials
	PublisherIfllImpactXoff  = 100 // ICE Europe Financials - Off-Market Trades
	PublisherIfusImpact      = 97  // ICE Futures US
	PublisherIfusImpactXoff  = 98  // ICE Futures US - Off-Market Trades
	PublisherMemxMemoir      = 15  // MEMX Memoir Depth
	PublisherNdexImpact      = 58  // ICE Endex
	PublisherNdexImpactXoff  = 85  // ICE Endex - Off-Market Trades
	PublisherOceaMemoir      = 107 // Blue Ocean ATS MEMOIR
	PublisherOpraPillar      = 30  // OPRA - Options Price Reporting Authority
	PublisherOpraPillarAmxo  = 20  // OPRA - NYSE American Options
	PublisherOpraPillarArco  = 29  // OPRA - NYSE Arca Options
	PublisherOpraPillarBato  = 36  // OPRA - Cboe BZX Options
	PublisherOpraPillarC2Ox  = 34  // OPRA - Cboe C2 Options
	PublisherOpraPillarEdgo  = 24  // OPRA - Cboe EDGX Options
	PublisherOpraPillarEmld  = 23  // OPRA - MIAX Emerald
	PublisherOpraPillarGmni  = 25  // OPRA - Nasdaq GEMX
	PublisherOpraPillarMcry  = 27  // OPRA - Nasdaq MRX
	PublisherOpraPillarMprl  = 31  // OPRA - MIAX Pearl
	PublisherOpraPillarMxop  = 37  // OPRA - MEMX Options
	PublisherOpraPillarSphr  = 61  // OPRA - MIAX Sapphire
	PublisherOpraPillarXbox  = 21  // OPRA - BOX Options
	PublisherOpraPillarXbxo  = 33  // OPRA - Nasdaq BX Options
	PublisherOpraPillarXcbo  = 22  // OPRA - Cboe Options
	PublisherOpraPillarXisx  = 26  // OPRA - Nasdaq ISE
	PublisherOpraPillarXmio  = 28  // OPRA - MIAX Options
	PublisherOpraPillarXndq  = 32  // OPRA - Nasdaq Options
	PublisherOpraPillarXphl  = 35  // OPRA - Nasdaq PHLX
	PublisherXasePillar      = 11  // NYSE American Integrated
	PublisherXbosItch        = 3   // Nasdaq BX TotalView-ITCH
	PublisherXcbfPitchXcbf   = 105 // Cboe Futures Exchange
	PublisherXcbfPitchXoff   = 106 // Cboe Futures Exchange - Off-Market Trades
	PublisherXchiPillar      = 12  // NYSE Texas Integrated
	PublisherXcisBbo         = 13  // NYSE National BBO
	PublisherXcisPillar      = 10  // NYSE National Integrated
	PublisherXcisTrades      = 14  // NYSE National Trades
	PublisherXcisTradesbbo   = 91  // NYSE National Trades and BBO
	PublisherXeeeEobiXeee    = 102 // European Energy Exchange EOBI
	PublisherXeeeEobiXoff    = 104 // European Energy Exchange EOBI - Off-Market Trades
	PublisherXeurEobiXeur    = 101 // Eurex EOBI
	PublisherXeurEobiXoff    = 103 // Eurex EOBI - Off-Market Trades
	PublisherXnasBasic       = 81  // Nasdaq Basic - Nasdaq
	PublisherXnasBasicEqus   = 93  // Nasdaq Basic - Consolidated
	PublisherXnasBasicFinc   = 83  // Nasdaq Basic - FINRA/Nasdaq TRF Chicago
	PublisherXnasBasicFinn   = 82  // Nasdaq Basic - FINRA/Nasdaq TRF Carteret
	PublisherXnasBasicXbos   = 88  // Nasdaq Basic - Nasdaq BX
	PublisherXnasBasicXpsx   = 89  // Nasdaq Basic - Nasdaq PSX
	PublisherXnasItch        = 2   // Nasdaq TotalView-ITCH (XNAS.ITCH)
	PublisherXnasNls         = 47  // Nasdaq Trades
	PublisherXnasNlsFinc     = 18  // FINRA/Nasdaq TRF Chicago
	PublisherXnasNlsFinn     = 17  // FINRA/Nasdaq TRF Carteret
	PublisherXnasNlsXbos     = 86  // Nasdaq NLS - Nasdaq BX
	PublisherXnasNlsXpsx     = 87  // Nasdaq NLS - Nasdaq PSX
	PublisherXnasQbbo        = 46  // Nasdaq QBBO
	PublisherXnysBbo         = 44  // NYSE BBO
	PublisherXnysPillar      = 9   // NYSE Integrated
	PublisherXnysTradesEqus  = 96  // NYSE Trades - Consolidated
	PublisherXnysTradesFiny  = 19  // FINRA/NYSE TRF
	PublisherXnysTradesXnys  = 45  // NYSE Trades
	PublisherXnysTradesbbo   = 92  // NYSE Trades and BBO
	PublisherXpsxItch        = 4   // Nasdaq PSX TotalView-ITCH
)

func (p Publisher) String() string {
	switch p {
	case 0:
		return "0"
	case PublisherGlbxMdp3:
		return "PublisherGlbxMdp3"
	case PublisherXnasItch:
		return "PublisherXnasItch"
	case PublisherXbosItch:
		return "PublisherXbosItch"
	case PublisherXpsxItch:
		return "PublisherXpsxItch"
	case PublisherBatsPitch:
		return "PublisherBatsPitch"
	case PublisherBatyPitch:
		return "PublisherBatyPitch"
	case PublisherEdgaPitch:
		return "PublisherEdgaPitch"
	case PublisherEdgxPitch:
		return "PublisherEdgxPitch"
	case PublisherXnysPillar:
		return "PublisherXnysPillar"
	case PublisherXcisPillar:
		return "PublisherXcisPillar"
	case PublisherXasePillar:
		return "PublisherXasePillar"
	case PublisherXchiPillar:
		return "PublisherXchiPillar"
	case PublisherXcisBbo:
		return "PublisherXcisBbo"
	case PublisherXcisTrades:
		return "PublisherXcisTrades"
	case PublisherMemxMemoir:
		return "PublisherMemxMemoir"
	case PublisherEprlDom:
		return "PublisherEprlDom"
	case PublisherXnasNlsFinn:
		return "PublisherXnasNlsFinn"
	case PublisherXnasNlsFinc:
		return "PublisherXnasNlsFinc"
	case PublisherXnysTradesFiny:
		return "PublisherXnysTradesFiny"
	case PublisherOpraPillarAmxo:
		return "PublisherOpraPillarAmxo"
	case PublisherOpraPillarXbox:
		return "PublisherOpraPillarXbox"
	case PublisherOpraPillarXcbo:
		return "PublisherOpraPillarXcbo"
	case PublisherOpraPillarEmld:
		return "PublisherOpraPillarEmld"
	case PublisherOpraPillarEdgo:
		return "PublisherOpraPillarEdgo"
	case PublisherOpraPillarGmni:
		return "PublisherOpraPillarGmni"
	case PublisherOpraPillarXisx:
		return "PublisherOpraPillarXisx"
	case PublisherOpraPillarMcry:
		return "PublisherOpraPillarMcry"
	case PublisherOpraPillarXmio:
		return "PublisherOpraPillarXmio"
	case PublisherOpraPillarArco:
		return "PublisherOpraPillarArco"
	case PublisherOpraPillar:
		return "PublisherOpraPillar"
	case PublisherOpraPillarMprl:
		return "PublisherOpraPillarMprl"
	case PublisherOpraPillarXndq:
		return "PublisherOpraPillarXndq"
	case PublisherOpraPillarXbxo:
		return "PublisherOpraPillarXbxo"
	case PublisherOpraPillarC2Ox:
		return "PublisherOpraPillarC2Ox"
	case PublisherOpraPillarXphl:
		return "PublisherOpraPillarXphl"
	case PublisherOpraPillarBato:
		return "PublisherOpraPillarBato"
	case PublisherOpraPillarMxop:
		return "PublisherOpraPillarMxop"
	case PublisherIexgTopsIexg:
		return "PublisherIexgTopsIexg"
	case PublisherDbeqBasicXchi:
		return "PublisherDbeqBasicXchi"
	case PublisherDbeqBasicXcis:
		return "PublisherDbeqBasicXcis"
	case PublisherDbeqBasicIexg:
		return "PublisherDbeqBasicIexg"
	case PublisherDbeqBasicEprl:
		return "PublisherDbeqBasicEprl"
	case PublisherArcxPillar:
		return "PublisherArcxPillar"
	case PublisherXnysBbo:
		return "PublisherXnysBbo"
	case PublisherXnysTradesXnys:
		return "PublisherXnysTradesXnys"
	case PublisherXnasQbbo:
		return "PublisherXnasQbbo"
	case PublisherXnasNls:
		return "PublisherXnasNls"
	case PublisherEqusPlusXchi:
		return "PublisherEqusPlusXchi"
	case PublisherEqusPlusXcis:
		return "PublisherEqusPlusXcis"
	case PublisherEqusPlusIexg:
		return "PublisherEqusPlusIexg"
	case PublisherEqusPlusEprl:
		return "PublisherEqusPlusEprl"
	case PublisherEqusPlusXnas:
		return "PublisherEqusPlusXnas"
	case PublisherEqusPlusXnys:
		return "PublisherEqusPlusXnys"
	case PublisherEqusPlusFinn:
		return "PublisherEqusPlusFinn"
	case PublisherEqusPlusFiny:
		return "PublisherEqusPlusFiny"
	case PublisherEqusPlusFinc:
		return "PublisherEqusPlusFinc"
	case PublisherIfeuImpact:
		return "PublisherIfeuImpact"
	case PublisherNdexImpact:
		return "PublisherNdexImpact"
	case PublisherDbeqBasic:
		return "PublisherDbeqBasic"
	case PublisherEqusPlusEqus:
		return "PublisherEqusPlusEqus"
	case PublisherOpraPillarSphr:
		return "PublisherOpraPillarSphr"
	case PublisherEqusAllXchi:
		return "PublisherEqusAllXchi"
	case PublisherEqusAllXcis:
		return "PublisherEqusAllXcis"
	case PublisherEqusAllIexg:
		return "PublisherEqusAllIexg"
	case PublisherEqusAllEprl:
		return "PublisherEqusAllEprl"
	case PublisherEqusAllXnas:
		return "PublisherEqusAllXnas"
	case PublisherEqusAllXnys:
		return "PublisherEqusAllXnys"
	case PublisherEqusAllFinn:
		return "PublisherEqusAllFinn"
	case PublisherEqusAllFiny:
		return "PublisherEqusAllFiny"
	case PublisherEqusAllFinc:
		return "PublisherEqusAllFinc"
	case PublisherEqusAllBats:
		return "PublisherEqusAllBats"
	case PublisherEqusAllBaty:
		return "PublisherEqusAllBaty"
	case PublisherEqusAllEdga:
		return "PublisherEqusAllEdga"
	case PublisherEqusAllEdgx:
		return "PublisherEqusAllEdgx"
	case PublisherEqusAllXbos:
		return "PublisherEqusAllXbos"
	case PublisherEqusAllXpsx:
		return "PublisherEqusAllXpsx"
	case PublisherEqusAllMemx:
		return "PublisherEqusAllMemx"
	case PublisherEqusAllXase:
		return "PublisherEqusAllXase"
	case PublisherEqusAllArcx:
		return "PublisherEqusAllArcx"
	case PublisherEqusAllLtse:
		return "PublisherEqusAllLtse"
	case PublisherXnasBasic:
		return "PublisherXnasBasic"
	case PublisherXnasBasicFinn:
		return "PublisherXnasBasicFinn"
	case PublisherXnasBasicFinc:
		return "PublisherXnasBasicFinc"
	case PublisherIfeuImpactXoff:
		return "PublisherIfeuImpactXoff"
	case PublisherNdexImpactXoff:
		return "PublisherNdexImpactXoff"
	case PublisherXnasNlsXbos:
		return "PublisherXnasNlsXbos"
	case PublisherXnasNlsXpsx:
		return "PublisherXnasNlsXpsx"
	case PublisherXnasBasicXbos:
		return "PublisherXnasBasicXbos"
	case PublisherXnasBasicXpsx:
		return "PublisherXnasBasicXpsx"
	case PublisherEqusSummaryEqus:
		return "PublisherEqusSummaryEqus"
	case PublisherXcisTradesbbo:
		return "PublisherXcisTradesbbo"
	case PublisherXnysTradesbbo:
		return "PublisherXnysTradesbbo"
	case PublisherXnasBasicEqus:
		return "PublisherXnasBasicEqus"
	case PublisherEqusAll:
		return "PublisherEqusAll"
	case PublisherEqusMini:
		return "PublisherEqusMini"
	case PublisherXnysTradesEqus:
		return "PublisherXnysTradesEqus"
	case PublisherIfusImpact:
		return "PublisherIfusImpact"
	case PublisherIfusImpactXoff:
		return "PublisherIfusImpactXoff"
	case PublisherIfllImpact:
		return "PublisherIfllImpact"
	case PublisherIfllImpactXoff:
		return "PublisherIfllImpactXoff"
	case PublisherXeurEobiXeur:
		return "PublisherXeurEobiXeur"
	case PublisherXeeeEobiXeee:
		return "PublisherXeeeEobiXeee"
	case PublisherXeurEobiXoff:
		return "PublisherXeurEobiXoff"
	case PublisherXeeeEobiXoff:
		return "PublisherXeeeEobiXoff"
	case PublisherXcbfPitchXcbf:
		return "PublisherXcbfPitchXcbf"
	case PublisherXcbfPitchXoff:
		return "PublisherXcbfPitchXoff"
	case PublisherOceaMemoir:
		return "PublisherOceaMemoir"
	default:
		return "Publisher(" + strconv.FormatUint(uint64(p), 10) + ")"
	}
}

func (p Publisher) GoString() string {
	return p.String()
}
