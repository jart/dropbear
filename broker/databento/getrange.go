package databento

import (
	"dropbear/clocky"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
)

type GetRangeParams struct {
	Dataset  string
	Schema   Schema
	STypeIn  SType // defaults to STypeInstrumentID (databento's recommended default is STypeRawSymbol)
	STypeOut SType // defaults to STypeInstrumentID
	Symbols  []string
	Start    clocky.Time
	End      clocky.Time
	Limit    int
}

type GetRangeResponse struct {
	Metadata Metadata
	reader   *zstd.Decoder
	body     io.ReadCloser
}

// GetRange fetches records from the historical timeseries API.
// Returns the parsed metadata and a slice of raw record byte slices.
// Each record slice contains the full record bytes including the header.
func (c *HistoricalClient) GetRange(params GetRangeParams) (*GetRangeResponse, error) {
	urlValues := url.Values{
		"dataset":     {params.Dataset},
		"schema":      {params.Schema.String()},
		"stype_in":    {params.STypeIn.String()},
		"stype_out":   {params.STypeOut.String()},
		"symbols":     {strings.Join(params.Symbols, ",")},
		"start":       {strconv.FormatInt(int64(params.Start), 10)},
		"encoding":    {"dbn"},
		"compression": {"zstd"},
	}
	if params.Limit > 0 {
		urlValues.Set("limit", fmt.Sprintf("%d", params.Limit))
	}
	if params.End != 0 {
		urlValues.Set("end", strconv.FormatInt(int64(params.End), 10))
	}
	encoded := urlValues.Encode()
	req, err := http.NewRequest("POST", historicalGateway+"/v0/timeseries.get_range",
		strings.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("databento: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(string(c.apiKey), "")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(encoded)), nil
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("databento: request failed with %d %s", resp.StatusCode, resp.Status)
	}
	zreader, err := zstd.NewReader(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("databento: zstd init: %w", err)
	}
	r := &GetRangeResponse{
		body:   resp.Body,
		reader: zreader,
	}
	err = decodeMetadata(r.reader, &r.Metadata)
	if err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

// Read reads the next record from the response.
func (r *GetRangeResponse) Read() (any, error) {
	rec, err := decodeRecord(r.reader)
	if err != nil {
		return nil, err
	}
	obj, err := castRecord(r.Metadata.Version, rec)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// Close closes the response body and releases resources.
func (r *GetRangeResponse) Close() error {
	r.reader.Close()
	err := r.body.Close()
	r.body = nil
	return err
}
