package alpaca

import "fmt"

type AssetStatus uint8

const (
	AssetStatusActive AssetStatus = iota
	AssetStatusInactive
)

func ParseAssetStatus(s string) (AssetStatus, error) {
	switch s {
	case "active":
		return AssetStatusActive, nil
	case "inactive":
		return AssetStatusInactive, nil
	default:
		return 0, fmt.Errorf("unknown asset status: %s", s)
	}
}

func (as AssetStatus) String() string {
	switch as {
	case AssetStatusActive:
		return "active"
	case AssetStatusInactive:
		return "inactive"
	default:
		panic("unknown asset status")
	}
}
