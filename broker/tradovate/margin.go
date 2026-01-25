package tradovate

import "dropbear/decimal"

type MarginRate struct {
	Day     decimal.Decimal
	Initial decimal.Decimal
}

var MarginRates = map[string]*MarginRate{
	"BCH": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("356.40"),
	},
	"BIP": {
		Day:     decimal.Parse("130.00"),
		Initial: decimal.Parse("260.00"),
	},
	"BIT": {
		Day:     decimal.Parse("25.00"),
		Initial: decimal.Parse("379.50"),
	},
	"BTC": {
		Day:     decimal.Parse("20000.00"),
		Initial: decimal.Parse("118067.40"),
	},
	"BTI": {
		Day:     decimal.Parse("10000.00"),
		Initial: decimal.Parse("38427.40"),
	},
	"DOG": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("1307.90"),
	},
	"ET": {
		Day:     decimal.Parse("25.00"),
		Initial: decimal.Parse("162.80"),
	},
	"ETH": {
		Day:     decimal.Parse("10000.00"),
		Initial: decimal.Parse("51717.60"),
	},
	"ETI": {
		Day:     decimal.Parse("20000.00"),
		Initial: decimal.Parse("16289.90"),
	},
	"ETP": {
		Day:     decimal.Parse("85.00"),
		Initial: decimal.Parse("170.00"),
	},
	"GSOL": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("29672.50"),
	},
	"LC": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("302.50"),
	},
	"M2K": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("1041.90"),
	},
	"MBT": {
		Day:     decimal.Parse("100.00"),
		Initial: decimal.Parse("2361.70"),
	},
	"MES": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("2500.31"),
	},
	"MET": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("104.50"),
	},
	"MMC": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("2518.78"),
	},
	"MNK": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("1977.83"),
	},
	"MNQ": {
		Day:     decimal.Parse("100.00"),
		Initial: decimal.Parse("3692.90"),
	},
	"MSC": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("1291.42"),
	},
	"MSL": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("1483.90"),
	},
	"MXP": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("2330.90"),
	},
	"MYM": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("1565.73"),
	},
	"SOL": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("605.00"),
	},
	"XRP": {
		Day:     decimal.Parse("345.00"),
		Initial: decimal.Parse("690.00"),
	},
	"ES": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("25003.07"),
	},
	"NQ": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("36928.97"),
	},
	"RTY": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("10419.00"),
	},
	"YM": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("15657.33"),
	},
	"B5": {
		Day:     decimal.Parse("70.40"),
		Initial: decimal.Parse("140.80"),
	},
	"EMD": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("20240.00"),
	},
	"FDAX": {
		Day:     decimal.Parse("2000.00"),
		Initial: decimal.Parse("45964.00"),
	},
	"FDXM": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("9192.00"),
	},
	"FDXS": {
		Day:     decimal.Parse("100.00"),
		Initial: decimal.Parse("1838.00"),
	},
	"FESX": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("4183.00"),
	},
	"FSXE": {
		Day:     decimal.Parse("209.00"),
		Initial: decimal.Parse("418.00"),
	},
	"FVS": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("393.00"),
	},
	"FXXP": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("2091.00"),
	},
	"GXRP": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("50662.00"),
	},
	"LB5": {
		Day:     decimal.Parse("704.00"),
		Initial: decimal.Parse("1408.00"),
	},
	"LTEC": {
		Day:     decimal.Parse("3843.40"),
		Initial: decimal.Parse("7686.80"),
	},
	"MC": {
		Day:     decimal.Parse("165.00"),
		Initial: decimal.Parse("330.00"),
	},
	"MFS": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("7840.00"),
	},
	"MME": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("4722.00"),
	},
	"NKD": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("14300.00"),
	},
	"SPK": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("9185.00"),
	},
	"SR1": {
		Day:     decimal.Parse("302.50"),
		Initial: decimal.Parse("605.00"),
	},
	"TEC": {
		Day:     decimal.Parse("384.45"),
		Initial: decimal.Parse("768.90"),
	},
	"BZ": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("4446.66"),
	},
	"CL": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("4626.57"),
	},
	"HO": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("6526.88"),
	},
	"MCL": {
		Day:     decimal.Parse("100.00"),
		Initial: decimal.Parse("464.86"),
	},
	"MNG": {
		Day:     decimal.Parse("100.00"),
		Initial: decimal.Parse("772.31"),
	},
	"NG": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("7701.14"),
	},
	"NOL": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("79.20"),
	},
	"QG": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1930.79"),
	},
	"QH": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("3268.07"),
	},
	"QM": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("2324.29"),
	},
	"RB": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("4997.77"),
	},
	"6A": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("2420.00"),
	},
	"6B": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("2200.00"),
	},
	"6C": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1210.00"),
	},
	"6E": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("3190.00"),
	},
	"6J": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("3410.00"),
	},
	"6L": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1210.00"),
	},
	"6M": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1430.00"),
	},
	"6N": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1540.00"),
	},
	"6S": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("4950.00"),
	},
	"DX": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("2186.00"),
	},
	"E7": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1485.00"),
	},
	"J7": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1540.00"),
	},
	"M6A": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("209.00"),
	},
	"M6B": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("220.00"),
	},
	"M6E": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("297.00"),
	},
	"MCD": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("110.00"),
	},
	"MJY": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("308.00"),
	},
	"MSF": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("495.00"),
	},
	"10Y": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("330.00"),
	},
	"2YY": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("363.00"),
	},
	"30Y": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("297.00"),
	},
	"5YY": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("341.00"),
	},
	"FGBL": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("1962.00"),
	},
	"FGBM": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("1067.00"),
	},
	"FGBS": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("389.00"),
	},
	"FGBX": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("3854.00"),
	},
	"GE": {
		Day:     decimal.Parse("478.50"),
		Initial: decimal.Parse("957.00"),
	},
	"MTN": {
		Day:     decimal.Parse("140.25"),
		Initial: decimal.Parse("280.50"),
	},
	"MWN": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("566.50"),
	},
	"TN": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("2805.00"),
	},
	"TWE": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("4730.00"),
	},
	"UB": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("5665.00"),
	},
	"Z3N": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("2000.00"),
	},
	"ZB": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("4070.00"),
	},
	"ZF": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1375.00"),
	},
	"ZN": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("2062.50"),
	},
	"ZQ": {
		Day:     decimal.Parse("728.75"),
		Initial: decimal.Parse("1457.50"),
	},
	"ZT": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1320.00"),
	},
	"1OZ": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("270.60"),
	},
	"GC": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("26998.40"),
	},
	"GOL": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("145.20"),
	},
	"HG": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("11000.00"),
	},
	"MGC": {
		Day:     decimal.Parse("100.00"),
		Initial: decimal.Parse("2702.70"),
	},
	"MHG": {
		Day:     decimal.Parse("200.00"),
		Initial: decimal.Parse("1100.00"),
	},
	"PA": {
		Day:     decimal.Parse("2000.00"),
		Initial: decimal.Parse("23115.40"),
	},
	"PL": {
		Day:     decimal.Parse("2000.00"),
		Initial: decimal.Parse("12681.90"),
	},
	"QC": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("5500.00"),
	},
	"QI": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("23852.40"),
	},
	"QO": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("13512.40"),
	},
	"SI": {
		Day:     decimal.Parse("2000.00"),
		Initial: decimal.Parse("47509.00"),
	},
	"SIL": {
		Day:     decimal.Parse("400.00"),
		Initial: decimal.Parse("9501.80"),
	},
	"YG": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("2195.00"),
	},
	"YI": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("3056.00"),
	},
	"MZC": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("107.80"),
	},
	"MZL": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("231.00"),
	},
	"MZM": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("170.50"),
	},
	"MZS": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("220.00"),
	},
	"MZW": {
		Day:     decimal.Parse("50.00"),
		Initial: decimal.Parse("181.50"),
	},
	"XC": {
		Day:     decimal.Parse("250.00"),
		Initial: decimal.Parse("214.50"),
	},
	"XK": {
		Day:     decimal.Parse("250.00"),
		Initial: decimal.Parse("440.00"),
	},
	"XW": {
		Day:     decimal.Parse("250.00"),
		Initial: decimal.Parse("363.00"),
	},
	"ZC": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1072.50"),
	},
	"ZL": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("2310.00"),
	},
	"ZM": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("1705.00"),
	},
	"ZO": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("1375.00"),
	},
	"ZR": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("1375.00"),
	},
	"ZS": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("2200.00"),
	},
	"ZW": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("1815.00"),
	},
	"GF": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("5830.00"),
	},
	"HE": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("1870.00"),
	},
	"LE": {
		Day:     decimal.Parse("500.00"),
		Initial: decimal.Parse("3410.00"),
	},
	"CC": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("7084.00"),
	},
	"CT": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("1254.00"),
	},
	"KC": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("9895.00"),
	},
	"LBR": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("1210.00"),
	},
	"LBS": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("5940.00"),
	},
	"OJ": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("4775.00"),
	},
	"SB": {
		Day:     decimal.Parse("1000.00"),
		Initial: decimal.Parse("837.00"),
	},
}
