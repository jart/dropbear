// HTTP client for Databento's historical API.
// Source: ~/vendor/databento-cpp/src/historical.cpp
// Source: ~/vendor/databento-cpp/src/detail/http_client.cpp
package databento

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unsafe"

	"dropbear/clocky"
)

const historicalGateway = "https://hist.databento.com"

// HistoricalClient accesses Databento's historical market data API.
type HistoricalClient struct {
	apiKey ApiKey
}

// NewHistoricalClient creates a new historical API client.
func NewHistoricalClient(apiKey ApiKey) *HistoricalClient {
	return &HistoricalClient{apiKey: apiKey}
}

// GetRange fetches records from the historical timeseries API.
// Returns the parsed metadata and a slice of raw record byte slices.
// Each record slice contains the full record bytes including the header.
func (c *HistoricalClient) GetRange(dataset string, schema Schema, stypeIn SType,
	symbols []string, start, end string) (Metadata, [][]byte, error) {

	params := url.Values{
		"dataset":     {dataset},
		"schema":      {schema.String()},
		"stype_in":    {stypeIn.String()},
		"stype_out":   {STypeInstrumentID.String()},
		"symbols":     {strings.Join(symbols, ",")},
		"start":       {start},
		"end":         {end},
		"encoding":    {"dbn"},
		"compression": {"none"},
	}

	req, err := http.NewRequest("POST", historicalGateway+"/v0/timeseries.get_range",
		strings.NewReader(params.Encode()))
	if err != nil {
		return Metadata{}, nil, fmt.Errorf("databento: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(string(c.apiKey), "")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Metadata{}, nil, fmt.Errorf("databento: http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return Metadata{}, nil, fmt.Errorf("databento: http %d: %s", resp.StatusCode, body)
	}

	body := bufio.NewReaderSize(resp.Body, 256*1024)
	meta, err := DecodeMetadata(body)
	if err != nil {
		return Metadata{}, nil, err
	}

	var records [][]byte
	for {
		rec, err := DecodeRecord(body)
		if err != nil {
			if err == io.EOF {
				break
			}
			return meta, records, err
		}
		rec, err = UpgradeRecord(meta.Version, rec)
		if err != nil {
			return meta, records, err
		}
		records = append(records, rec)
	}

	return meta, records, nil
}

// GetInstruments fetches instrument definitions for a parent symbol on a given
// date via the historical API. Returns a slice of InstrumentRef backed by the
// raw record bytes.
func GetInstruments(apiKey ApiKey, dataset, symbol string, date clocky.Time) ([]InstrumentRef, error) {
	y, m, d := date.Date()
	start := fmt.Sprintf("%04d-%02d-%02dT00:00:00Z", y, m, d)
	// Use end of trading session (17:30 UTC) rather than next day midnight,
	// since the historical API rejects end times past available data.
	end := fmt.Sprintf("%04d-%02d-%02dT17:30:00Z", y, m, d)
	client := NewHistoricalClient(apiKey)
	_, records, err := client.GetRange(dataset, SchemaDefinition,
		STypeParent, []string{symbol}, start, end)
	if err != nil {
		return nil, err
	}
	instSize := int(unsafe.Sizeof(Instrument{}))
	var result []InstrumentRef
	for _, rec := range records {
		if len(rec) < instSize {
			continue
		}
		if RType(rec[1]) != RTypeInstrumentDef {
			continue
		}
		result = append(result, InstrumentRef{
			Instrument: (*Instrument)(unsafe.Pointer(&rec[0])),
			raw:        rec,
		})
	}
	return result, nil
}

// InstrumentRef holds an Instrument pointer and the backing raw bytes.
type InstrumentRef struct {
	*Instrument
	raw []byte // prevents GC of the backing record
}
