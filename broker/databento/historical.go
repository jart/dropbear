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
	symbols []string, start, end string, limit int) (*Metadata, []any, error) {

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
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

	req, err := http.NewRequest("POST", historicalGateway+"/v0/timeseries.get_range",
		strings.NewReader(params.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("databento: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(string(c.apiKey), "")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("databento: http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("databento: http %d: %s", resp.StatusCode, body)
	}

	body := bufio.NewReaderSize(resp.Body, 256*1024)
	var meta Metadata
	err = decodeMetadata(body, &meta)
	if err != nil {
		return nil, nil, err
	}

	var records []any
	for {
		rec, err := decodeRecord(body)
		if err != nil {
			if err == io.EOF {
				break
			}
			return &meta, records, err
		}
		obj, err := castRecord(meta.Version, rec)
		if err != nil {
			return &meta, records, err
		}
		records = append(records, obj)
	}
	return &meta, records, nil
}
