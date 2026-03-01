package databento

import "strconv"

type Publisher uint16

const (
	PublisherGlbxMdp3Glbx      = 1   // CME Globex MDP 3.0
	PublisherXnasItchXnas      = 2   // Nasdaq TotalView-ITCH
	PublisherXbosItchXbos      = 3   // Nasdaq BX TotalView-ITCH
	PublisherXpsxItchXpsx      = 4   // Nasdaq PSX TotalView-ITCH
	PublisherBatsPitchBats     = 5   // Cboe BZX Depth
	PublisherBatyPitchBaty     = 6   // Cboe BYX Depth
	PublisherEdgaPitchEdga     = 7   // Cboe EDGA Depth
	PublisherEdgxPitchEdgx     = 8   // Cboe EDGX Depth
	PublisherXnysPillarXnys    = 9   // NYSE Integrated
	PublisherXcisPillarXcis    = 10  // NYSE National Integrated
	PublisherXasePillarXase    = 11  // NYSE American Integrated
	PublisherXchiPillarXchi    = 12  // NYSE Texas Integrated
	PublisherXcisBboXcis       = 13  // NYSE National BBO
	PublisherXcisTradesXcis    = 14  // NYSE National Trades
	PublisherMemxMemoirMemx    = 15  // MEMX Memoir Depth
	PublisherEprlDomEprl       = 16  // MIAX Pearl Depth
	PublisherXnasNlsFinn       = 17  // FINRA/Nasdaq TRF Carteret
	PublisherXnasNlsFinc       = 18  // FINRA/Nasdaq TRF Chicago
	PublisherXnysTradesFiny    = 19  // FINRA/NYSE TRF
	PublisherOpraPillarAmxo    = 20  // OPRA - NYSE American Options
	PublisherOpraPillarXbox    = 21  // OPRA - BOX Options
	PublisherOpraPillarXcbo    = 22  // OPRA - Cboe Options
	PublisherOpraPillarEmld    = 23  // OPRA - MIAX Emerald
	PublisherOpraPillarEdgo    = 24  // OPRA - Cboe EDGX Options
	PublisherOpraPillarGmni    = 25  // OPRA - Nasdaq GEMX
	PublisherOpraPillarXisx    = 26  // OPRA - Nasdaq ISE
	PublisherOpraPillarMcry    = 27  // OPRA - Nasdaq MRX
	PublisherOpraPillarXmio    = 28  // OPRA - MIAX Options
	PublisherOpraPillarArco    = 29  // OPRA - NYSE Arca Options
	PublisherOpraPillarOpra    = 30  // OPRA - Options Price Reporting Authority
	PublisherOpraPillarMprl    = 31  // OPRA - MIAX Pearl
	PublisherOpraPillarXndq    = 32  // OPRA - Nasdaq Options
	PublisherOpraPillarXbxo    = 33  // OPRA - Nasdaq BX Options
	PublisherOpraPillarC2Ox    = 34  // OPRA - Cboe C2 Options
	PublisherOpraPillarXphl    = 35  // OPRA - Nasdaq PHLX
	PublisherOpraPillarBato    = 36  // OPRA - Cboe BZX Options
	PublisherOpraPillarMxop    = 37  // OPRA - MEMX Options
	PublisherIexgTopsIexg      = 38  // IEX TOPS
	PublisherDbeqBasicXchi     = 39  // DBEQ Basic - NYSE Texas
	PublisherDbeqBasicXcis     = 40  // DBEQ Basic - NYSE National
	PublisherDbeqBasicIexg     = 41  // DBEQ Basic - IEX
	PublisherDbeqBasicEprl     = 42  // DBEQ Basic - MIAX Pearl
	PublisherArcxPillarArcx    = 43  // NYSE Arca Integrated
	PublisherXnysBboXnys       = 44  // NYSE BBO
	PublisherXnysTradesXnys    = 45  // NYSE Trades
	PublisherXnasQbboXnas      = 46  // Nasdaq QBBO
	PublisherXnasNlsXnas       = 47  // Nasdaq Trades
	PublisherEqusPlusXchi      = 48  // Databento US Equities Plus - NYSE Texas
	PublisherEqusPlusXcis      = 49  // Databento US Equities Plus - NYSE National
	PublisherEqusPlusIexg      = 50  // Databento US Equities Plus - IEX
	PublisherEqusPlusEprl      = 51  // Databento US Equities Plus - MIAX Pearl
	PublisherEqusPlusXnas      = 52  // Databento US Equities Plus - Nasdaq
	PublisherEqusPlusXnys      = 53  // Databento US Equities Plus - NYSE
	PublisherEqusPlusFinn      = 54  // Databento US Equities Plus - FINRA/Nasdaq TRF Carteret
	PublisherEqusPlusFiny      = 55  // Databento US Equities Plus - FINRA/NYSE TRF
	PublisherEqusPlusFinc      = 56  // Databento US Equities Plus - FINRA/Nasdaq TRF Chicago
	PublisherIfeuImpactIfeu    = 57  // ICE Europe Commodities
	PublisherNdexImpactNdex    = 58  // ICE Endex
	PublisherDbeqBasicDbeq     = 59  // Databento US Equities Basic - Consolidated
	PublisherEqusPlusEqus      = 60  // EQUS Plus - Consolidated
	PublisherOpraPillarSphr    = 61  // OPRA - MIAX Sapphire
	PublisherEqusAllXchi       = 62  // Databento US Equities (All Feeds) - NYSE Texas
	PublisherEqusAllXcis       = 63  // Databento US Equities (All Feeds) - NYSE National
	PublisherEqusAllIexg       = 64  // Databento US Equities (All Feeds) - IEX
	PublisherEqusAllEprl       = 65  // Databento US Equities (All Feeds) - MIAX Pearl
	PublisherEqusAllXnas       = 66  // Databento US Equities (All Feeds) - Nasdaq
	PublisherEqusAllXnys       = 67  // Databento US Equities (All Feeds) - NYSE
	PublisherEqusAllFinn       = 68  // Databento US Equities (All Feeds) - FINRA/Nasdaq TRF Carteret
	PublisherEqusAllFiny       = 69  // Databento US Equities (All Feeds) - FINRA/NYSE TRF
	PublisherEqusAllFinc       = 70  // Databento US Equities (All Feeds) - FINRA/Nasdaq TRF Chicago
	PublisherEqusAllBats       = 71  // Databento US Equities (All Feeds) - Cboe BZX
	PublisherEqusAllBaty       = 72  // Databento US Equities (All Feeds) - Cboe BYX
	PublisherEqusAllEdga       = 73  // Databento US Equities (All Feeds) - Cboe EDGA
	PublisherEqusAllEdgx       = 74  // Databento US Equities (All Feeds) - Cboe EDGX
	PublisherEqusAllXbos       = 75  // Databento US Equities (All Feeds) - Nasdaq BX
	PublisherEqusAllXpsx       = 76  // Databento US Equities (All Feeds) - Nasdaq PSX
	PublisherEqusAllMemx       = 77  // Databento US Equities (All Feeds) - MEMX
	PublisherEqusAllXase       = 78  // Databento US Equities (All Feeds) - NYSE American
	PublisherEqusAllArcx       = 79  // Databento US Equities (All Feeds) - NYSE Arca
	PublisherEqusAllLtse       = 80  // Databento US Equities (All Feeds) - Long-Term Stock Exchange
	PublisherXnasBasicXnas     = 81  // Nasdaq Basic - Nasdaq
	PublisherXnasBasicFinn     = 82  // Nasdaq Basic - FINRA/Nasdaq TRF Carteret
	PublisherXnasBasicFinc     = 83  // Nasdaq Basic - FINRA/Nasdaq TRF Chicago
	PublisherIfeuImpactXoff    = 84  // ICE Europe - Off-Market Trades
	PublisherNdexImpactXoff    = 85  // ICE Endex - Off-Market Trades
	PublisherXnasNlsXbos       = 86  // Nasdaq NLS - Nasdaq BX
	PublisherXnasNlsXpsx       = 87  // Nasdaq NLS - Nasdaq PSX
	PublisherXnasBasicXbos     = 88  // Nasdaq Basic - Nasdaq BX
	PublisherXnasBasicXpsx     = 89  // Nasdaq Basic - Nasdaq PSX
	PublisherEqusSummaryEqus   = 90  // Databento Equities Summary
	PublisherXcisTradesbboXcis = 91  // NYSE National Trades and BBO
	PublisherXnysTradesbboXnys = 92  // NYSE Trades and BBO
	PublisherXnasBasicEqus     = 93  // Nasdaq Basic - Consolidated
	PublisherEqusAllEqus       = 94  // Databento US Equities (All Feeds) - Consolidated
	PublisherEqusMiniEqus      = 95  // Databento US Equities Mini
	PublisherXnysTradesEqus    = 96  // NYSE Trades - Consolidated
	PublisherIfusImpactIfus    = 97  // ICE Futures US
	PublisherIfusImpactXoff    = 98  // ICE Futures US - Off-Market Trades
	PublisherIfllImpactIfll    = 99  // ICE Europe Financials
	PublisherIfllImpactXoff    = 100 // ICE Europe Financials - Off-Market Trades
	PublisherXeurEobiXeur      = 101 // Eurex EOBI
	PublisherXeeeEobiXeee      = 102 // European Energy Exchange EOBI
	PublisherXeurEobiXoff      = 103 // Eurex EOBI - Off-Market Trades
	PublisherXeeeEobiXoff      = 104 // European Energy Exchange EOBI - Off-Market Trades
	PublisherXcbfPitchXcbf     = 105 // Cboe Futures Exchange
	PublisherXcbfPitchXoff     = 106 // Cboe Futures Exchange - Off-Market Trades
	PublisherOceaMemoirOcea    = 107 // Blue Ocean ATS MEMOIR
)

func (p Publisher) String() string {
	switch p {
	case 0:
		return "0"
	case PublisherGlbxMdp3Glbx:
		return "PublisherGlbxMdp3Glbx"
	case PublisherXnasItchXnas:
		return "PublisherXnasItchXnas"
	case PublisherXbosItchXbos:
		return "PublisherXbosItchXbos"
	case PublisherXpsxItchXpsx:
		return "PublisherXpsxItchXpsx"
	case PublisherBatsPitchBats:
		return "PublisherBatsPitchBats"
	case PublisherBatyPitchBaty:
		return "PublisherBatyPitchBaty"
	case PublisherEdgaPitchEdga:
		return "PublisherEdgaPitchEdga"
	case PublisherEdgxPitchEdgx:
		return "PublisherEdgxPitchEdgx"
	case PublisherXnysPillarXnys:
		return "PublisherXnysPillarXnys"
	case PublisherXcisPillarXcis:
		return "PublisherXcisPillarXcis"
	case PublisherXasePillarXase:
		return "PublisherXasePillarXase"
	case PublisherXchiPillarXchi:
		return "PublisherXchiPillarXchi"
	case PublisherXcisBboXcis:
		return "PublisherXcisBboXcis"
	case PublisherXcisTradesXcis:
		return "PublisherXcisTradesXcis"
	case PublisherMemxMemoirMemx:
		return "PublisherMemxMemoirMemx"
	case PublisherEprlDomEprl:
		return "PublisherEprlDomEprl"
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
	case PublisherOpraPillarOpra:
		return "PublisherOpraPillarOpra"
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
	case PublisherArcxPillarArcx:
		return "PublisherArcxPillarArcx"
	case PublisherXnysBboXnys:
		return "PublisherXnysBboXnys"
	case PublisherXnysTradesXnys:
		return "PublisherXnysTradesXnys"
	case PublisherXnasQbboXnas:
		return "PublisherXnasQbboXnas"
	case PublisherXnasNlsXnas:
		return "PublisherXnasNlsXnas"
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
	case PublisherIfeuImpactIfeu:
		return "PublisherIfeuImpactIfeu"
	case PublisherNdexImpactNdex:
		return "PublisherNdexImpactNdex"
	case PublisherDbeqBasicDbeq:
		return "PublisherDbeqBasicDbeq"
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
	case PublisherXnasBasicXnas:
		return "PublisherXnasBasicXnas"
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
	case PublisherXcisTradesbboXcis:
		return "PublisherXcisTradesbboXcis"
	case PublisherXnysTradesbboXnys:
		return "PublisherXnysTradesbboXnys"
	case PublisherXnasBasicEqus:
		return "PublisherXnasBasicEqus"
	case PublisherEqusAllEqus:
		return "PublisherEqusAllEqus"
	case PublisherEqusMiniEqus:
		return "PublisherEqusMiniEqus"
	case PublisherXnysTradesEqus:
		return "PublisherXnysTradesEqus"
	case PublisherIfusImpactIfus:
		return "PublisherIfusImpactIfus"
	case PublisherIfusImpactXoff:
		return "PublisherIfusImpactXoff"
	case PublisherIfllImpactIfll:
		return "PublisherIfllImpactIfll"
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
	case PublisherOceaMemoirOcea:
		return "PublisherOceaMemoirOcea"
	default:
		return "Publisher(" + strconv.FormatUint(uint64(p), 10) + ")"
	}
}

func (p Publisher) GoString() string {
	return p.String()
}
