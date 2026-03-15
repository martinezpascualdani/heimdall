package client

import (
	"context"
	"fmt"
	"net/url"
)

// ScopeSyncResultItem is one result from POST /v1/imports/sync (scope-service).
type ScopeSyncResultItem struct {
	Registry        string `json:"registry"`
	DatasetID       string `json:"dataset_id"`
	Status          string `json:"status"`
	BlocksPersisted int64  `json:"blocks_persisted"`
	ASNsPersisted   int64  `json:"asns_persisted"`
	DurationMs      int64  `json:"duration_ms"`
	Error           string `json:"error,omitempty"`
}

// ScopeSyncResponse is the response of scope POST /v1/imports/sync.
type ScopeSyncResponse struct {
	Results []ScopeSyncResultItem `json:"results"`
}

// IPResolveResponse is GET /v1/scopes/by-ip/{ip} response.
// Serial is sent as number by scope-service; we accept int64 for unmarshaling.
type IPResolveResponse struct {
	IP         string `json:"ip"`
	ScopeType  string `json:"scope_type"`
	ScopeValue string `json:"scope_value"`
	DatasetID  string `json:"dataset_id"`
	Registry   string `json:"registry,omitempty"`
	Serial     int64  `json:"serial,omitempty"`
}

// CountrySummaryResponse is GET /v1/scopes/country/{cc}/summary response.
type CountrySummaryResponse struct {
	ScopeType    string `json:"scope_type"`
	ScopeValue   string `json:"scope_value"`
	DatasetsUsed []struct {
		DatasetID string `json:"dataset_id"`
		Registry  string `json:"registry"`
		Serial    string `json:"serial,omitempty"`
	} `json:"datasets_used"`
	IPv4BlockCount int64 `json:"ipv4_block_count"`
	IPv6BlockCount int64 `json:"ipv6_block_count"`
	Total          int64 `json:"total"`
	AddressFamily  string `json:"address_family,omitempty"`
}

// ScopeBlockItem is one block in blocks response.
type ScopeBlockItem struct {
	StartValue string `json:"start_value"`
	EndValue   string `json:"end_value"`
	Count      int64  `json:"count"`
	Status     string `json:"status,omitempty"`
	Date       string `json:"date,omitempty"`
}

// CountryBlocksResponse is GET /v1/scopes/country/{cc}/blocks response.
type CountryBlocksResponse struct {
	ScopeType    string            `json:"scope_type"`
	ScopeValue   string            `json:"scope_value"`
	DatasetsUsed []struct {
		DatasetID string `json:"dataset_id"`
		Registry  string `json:"registry"`
		Serial    string `json:"serial,omitempty"`
	} `json:"datasets_used"`
	AddressFamily string           `json:"address_family"`
	Count         int              `json:"count"`
	Total         int              `json:"total"`
	Limit         int              `json:"limit"`
	Offset        int              `json:"offset"`
	HasMore       bool             `json:"has_more"`
	Items         []ScopeBlockItem `json:"items"`
}

// ScopeASNItem is one ASN range in asns response.
type ScopeASNItem struct {
	ASNStart  int64  `json:"asn_start"`
	ASNEnd    int64  `json:"asn_end"`
	Registry  string `json:"registry,omitempty"`
	Date      string `json:"date,omitempty"`
	Status    string `json:"status,omitempty"`
}

// CountryASNsResponse is GET /v1/scopes/country/{cc}/asns response.
type CountryASNsResponse struct {
	ScopeType    string          `json:"scope_type"`
	ScopeValue   string          `json:"scope_value"`
	DatasetsUsed []struct {
		DatasetID string `json:"dataset_id"`
		Registry  string `json:"registry"`
	} `json:"datasets_used"`
	Count   int             `json:"count"`
	Total   int             `json:"total"`
	Limit   int             `json:"limit"`
	Offset  int             `json:"offset"`
	HasMore bool            `json:"has_more"`
	Items   []ScopeASNItem  `json:"items"`
}

// CountryASNSummaryResponse is GET /v1/scopes/country/{cc}/asn-summary response.
type CountryASNSummaryResponse struct {
	ScopeType     string `json:"scope_type"`
	ScopeValue    string `json:"scope_value"`
	ASNRangeCount int    `json:"asn_range_count"`
	ASNTotalCount int64  `json:"asn_total_count"`
	DatasetsUsed  []struct {
		DatasetID string `json:"dataset_id"`
		Registry  string `json:"registry"`
	} `json:"datasets_used"`
}

// CountryDatasetsResponse is GET /v1/scopes/country/{cc}/datasets response.
type CountryDatasetsResponse struct {
	ScopeType  string `json:"scope_type"`
	ScopeValue string `json:"scope_value"`
	Datasets   []struct {
		DatasetID string `json:"dataset_id"`
		Registry  string `json:"registry"`
		Serial    string `json:"serial,omitempty"`
	} `json:"datasets"`
}

// ScopeSync calls POST /v1/imports/sync (scope-service).
func (c *Client) ScopeSync(ctx context.Context) (*ScopeSyncResponse, error) {
	var out ScopeSyncResponse
	if err := c.post(ctx, c.scope, "/v1/imports/sync", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ScopeByIP calls GET /v1/scopes/by-ip/{ip}.
func (c *Client) ScopeByIP(ctx context.Context, ip, datasetID string) (*IPResolveResponse, error) {
	if ip == "" {
		return nil, fmt.Errorf("ip required")
	}
	path := "/v1/scopes/by-ip/" + url.PathEscape(ip)
	if datasetID != "" {
		path += "?dataset_id=" + url.QueryEscape(datasetID)
	}
	var out IPResolveResponse
	if err := c.get(ctx, c.scope, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ScopeCountrySummary calls GET /v1/scopes/country/{cc}/summary.
func (c *Client) ScopeCountrySummary(ctx context.Context, cc, datasetID, addressFamily string) (*CountrySummaryResponse, error) {
	if cc == "" {
		return nil, fmt.Errorf("country code required")
	}
	path := "/v1/scopes/country/" + url.PathEscape(cc) + "/summary"
	path = addQuery(path, datasetID, addressFamily, 0, 0)
	var out CountrySummaryResponse
	if err := c.get(ctx, c.scope, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ScopeCountryBlocks calls GET /v1/scopes/country/{cc}/blocks.
func (c *Client) ScopeCountryBlocks(ctx context.Context, cc, datasetID, addressFamily string, limit, offset int) (*CountryBlocksResponse, error) {
	if cc == "" {
		return nil, fmt.Errorf("country code required")
	}
	path := "/v1/scopes/country/" + url.PathEscape(cc) + "/blocks"
	path = addQuery(path, datasetID, addressFamily, limit, offset)
	var out CountryBlocksResponse
	if err := c.get(ctx, c.scope, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ScopeCountryASNs calls GET /v1/scopes/country/{cc}/asns.
func (c *Client) ScopeCountryASNs(ctx context.Context, cc, datasetID string, limit, offset int) (*CountryASNsResponse, error) {
	if cc == "" {
		return nil, fmt.Errorf("country code required")
	}
	path := "/v1/scopes/country/" + url.PathEscape(cc) + "/asns"
	if datasetID != "" {
		path += "?dataset_id=" + url.QueryEscape(datasetID)
	}
	if limit > 0 {
		if datasetID != "" {
			path += "&"
		} else {
			path += "?"
		}
		path += "limit=" + url.QueryEscape(fmt.Sprintf("%d", limit)) + "&offset=" + url.QueryEscape(fmt.Sprintf("%d", offset))
	}
	var out CountryASNsResponse
	if err := c.get(ctx, c.scope, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ScopeCountryASNSummary calls GET /v1/scopes/country/{cc}/asn-summary.
func (c *Client) ScopeCountryASNSummary(ctx context.Context, cc, datasetID string) (*CountryASNSummaryResponse, error) {
	if cc == "" {
		return nil, fmt.Errorf("country code required")
	}
	path := "/v1/scopes/country/" + url.PathEscape(cc) + "/asn-summary"
	if datasetID != "" {
		path += "?dataset_id=" + url.QueryEscape(datasetID)
	}
	var out CountryASNSummaryResponse
	if err := c.get(ctx, c.scope, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ScopeCountryDatasets calls GET /v1/scopes/country/{cc}/datasets.
func (c *Client) ScopeCountryDatasets(ctx context.Context, cc string) (*CountryDatasetsResponse, error) {
	if cc == "" {
		return nil, fmt.Errorf("country code required")
	}
	path := "/v1/scopes/country/" + url.PathEscape(cc) + "/datasets"
	var out CountryDatasetsResponse
	if err := c.get(ctx, c.scope, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func addQuery(path, datasetID, addressFamily string, limit, offset int) string {
	var q url.Values
	if datasetID != "" {
		q = url.Values{}
		q.Set("dataset_id", datasetID)
	}
	if addressFamily != "" {
		if q == nil {
			q = url.Values{}
		}
		q.Set("address_family", addressFamily)
	}
	if limit > 0 {
		if q == nil {
			q = url.Values{}
		}
		q.Set("limit", fmt.Sprintf("%d", limit))
		q.Set("offset", fmt.Sprintf("%d", offset))
	}
	if len(q) > 0 {
		return path + "?" + q.Encode()
	}
	return path
}
