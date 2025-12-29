package alpaca

import "fmt"

type AssetClass int

const (
	AssetClassUSEquity AssetClass = iota
	AssetClassCrypto
	AssetClassCryptoPerp
)

func ParseAssetClass(s string) (AssetClass, error) {
	switch s {
	case "us_equity":
		return AssetClassUSEquity, nil
	case "crypto":
		return AssetClassCrypto, nil
	case "crypto_perp":
		return AssetClassCryptoPerp, nil
	default:
		return 0, fmt.Errorf("unknown asset class: %s", s)
	}
}

func (ac AssetClass) String() string {
	switch ac {
	case AssetClassUSEquity:
		return "us_equity"
	case AssetClassCrypto:
		return "crypto"
	case AssetClassCryptoPerp:
		return "crypto_perp"
	default:
		panic("unknown asset class")
	}
}
