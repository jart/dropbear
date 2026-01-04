package alpaca

import (
	"fmt"
	"sync/atomic"
)

type AssetStatus int32

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

func (as AssetStatus) GoString() string {
	switch as {
	case AssetStatusActive:
		return "AssetStatusActive"
	case AssetStatusInactive:
		return "AssetStatusInactive"
	default:
		panic("unknown asset status")
	}
}

// Load atomically loads and returns the value of as.
func (as *AssetStatus) Load() AssetStatus {
	return AssetStatus(atomic.LoadInt32((*int32)(as)))
}

// Store atomically stores v into as.
func (as *AssetStatus) Store(v AssetStatus) {
	atomic.StoreInt32((*int32)(as), int32(v))
}

func (as AssetStatus) MarshalJSON() ([]byte, error) {
	return []byte(`"` + as.String() + `"`), nil
}

func (as *AssetStatus) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid asset status: %s", data)
	}
	v, err := ParseAssetStatus(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*as = v
	return nil
}
