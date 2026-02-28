package databento

import "dropbear/clocky"

// Metadata contains DBN file header information.
type Metadata struct {
	Version       uint8           // dbn schema version number
	Dataset       string          // dataset code
	Schema        Schema          // optional data record schema which affects type of record present
	Start         clocky.Time     // unix timestamp of query start, or first record if file was split
	End           clocky.Time     // unix timestamp of query end, or last record if file was split
	Limit         uint64          // maximum number of records for query
	STypeIn       SType           // input symbology type
	STypeOut      SType           // output symbology type
	TSOut         bool            // whether records contain appended send timestamp
	SymbolCstrLen uint16          // length in bytes of fixed-length symbol strings, including nul terminator byte
	Symbols       []string        // original queryinput symbols from request
	Partial       []string        // symbols that didn't resolve for at least one dayin query time range
	NotFound      []string        // symbols that didn't resolve for any day in query time range
	Mappings      []SymbolMapping // symbol mappings containing a native symbol and its mapping intervals
}

type SymbolMapping struct {
	RawSymbol string
	Intervals []MappingInterval
}

type MappingInterval struct {
	StartDate clocky.Time // inclusive
	EndDate   clocky.Time // exclusive
	Symbol    string      // resolved symbol for this interval (in stype_out)
}
