package schwab

import "fmt"

type AssetType uint8

const (
	AssetTypeEquity               AssetType = iota // stocks and ETFs
	AssetTypeOption                                // options contracts
	AssetTypeSweepVehicle                          // sweep vehicle (internal)
	AssetTypeCollectiveInvestment                  // mutual funds, money market funds
)

func ParseAssetType(s string) (AssetType, error) {
	switch s {
	case "EQUITY":
		return AssetTypeEquity, nil
	case "OPTION":
		return AssetTypeOption, nil
	case "SWEEP_VEHICLE":
		return AssetTypeSweepVehicle, nil
	case "COLLECTIVE_INVESTMENT":
		return AssetTypeCollectiveInvestment, nil
	default:
		return 0, fmt.Errorf("unknown asset type: %s", s)
	}
}

func (a AssetType) String() string {
	switch a {
	case AssetTypeEquity:
		return "EQUITY"
	case AssetTypeOption:
		return "OPTION"
	case AssetTypeSweepVehicle:
		return "SWEEP_VEHICLE"
	case AssetTypeCollectiveInvestment:
		return "COLLECTIVE_INVESTMENT"
	default:
		panic("unknown asset type")
	}
}

func (a AssetType) GoString() string {
	switch a {
	case AssetTypeEquity:
		return "AssetTypeEquity"
	case AssetTypeOption:
		return "AssetTypeOption"
	case AssetTypeSweepVehicle:
		return "AssetTypeSweepVehicle"
	case AssetTypeCollectiveInvestment:
		return "AssetTypeCollectiveInvestment"
	default:
		panic("unknown asset type")
	}
}

func (a AssetType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + a.String() + `"`), nil
}

func (a *AssetType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid asset type: %s", data)
	}
	v, err := ParseAssetType(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*a = v
	return nil
}
