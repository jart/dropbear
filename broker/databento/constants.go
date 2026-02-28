package databento

const (
	DBNMagic                   = "DBN"
	DBNVersion                 = 3 // current version of dbn encoding
	DatasetLen                 = 16
	SymbolCstrLen              = 71 // length of fixed-length symbol strings
	AssetCstrLen               = 11 // length of fixed-length asset strings
	MetadataPreludeSize        = 8
	FixedMetadataLen           = 100
	NullSType           uint8  = 0xff
	NullSchema          uint16 = 0xffff
	NullRecordCount     uint64 = 0xffffffffffffffff
	UndefPrice          int64  = 0x7fffffffffffffff
	UndefOrderSize      uint32 = 0xffffffff
	UndefStatQuantity   int64  = 0x7fffffffffffffff
	FixedPriceScale     int64  = 1000000000
)
