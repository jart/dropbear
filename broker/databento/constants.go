package databento

const (
	DBNMagic                   = "DBN"
	DBNVersion                 = 3 // current version of dbn encoding
	DatasetLen                 = 16
	SymbolCstrLen              = 71 // length of fixed-length symbol strings (v2+)
	AssetCstrLen               = 11 // length of fixed-length asset strings (v3)
	AssetCstrLenV2             = 7  // length of fixed-length asset strings (v1, v2)
	MetadataPreludeSize        = 8
	FixedMetadataLen           = 100
	NullSType           uint8  = 0xff
	NullSchema          uint16 = 0xffff
	NullRecordCount     uint64 = 0xffffffffffffffff
	UndefPrice          int64  = 0x7fffffffffffffff
	UndefOrderSize      uint32 = 0xffffffff
	UndefStatQuantity   int64  = 0x7fffffffffffffff
	UndefTimestamp      uint64 = 0xffffffffffffffff
	UndefInt32          int32  = 0x7fffffff
	UndefUint32         uint32 = 0xffffffff
	FixedPriceScale     int64  = 1000000000
)
