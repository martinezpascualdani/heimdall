package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// RoutingByIPResponse is GET /v1/asn/by-ip/{ip} response.
type RoutingByIPResponse struct {
	IP               string `json:"ip"`
	MatchedPrefix    string `json:"matched_prefix"`
	PrefixLength     int    `json:"prefix_length"`
	ASNRaw           string `json:"asn_raw"`
	PrimaryASN       *int64 `json:"primary_asn"`
	ASNType          string `json:"asn_type"`
	ASName           string `json:"as_name,omitempty"`
	OrgName          string `json:"org_name,omitempty"`
	RoutingDataset   string `json:"routing_dataset,omitempty"`
	MetadataDataset  string `json:"metadata_dataset,omitempty"`
}

// RoutingASNMetaResponse is GET /v1/asn/{asn} response.
type RoutingASNMetaResponse struct {
	ASN              int64  `json:"asn"`
	ASName           string `json:"as_name,omitempty"`
	OrgName          string `json:"org_name,omitempty"`
	Source           string `json:"source,omitempty"`
	SourceDatasetID  string `json:"source_dataset_id,omitempty"`
}

// PrefixItem is one prefix in ASN prefixes response.
type PrefixItem struct {
	Prefix       string `json:"prefix"`
	PrefixLength int    `json:"prefix_length"`
	ASNRaw       string `json:"asn_raw"`
	ASNType      string `json:"asn_type"`
}

// RoutingASNPrefixesResponse is GET /v1/asn/prefixes/{asn} response.
type RoutingASNPrefixesResponse struct {
	ASN        int64        `json:"asn"`
	Items      []PrefixItem `json:"items"`
	Total      int          `json:"total"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
	HasMore    bool         `json:"has_more"`
	DatasetID  string       `json:"dataset_id,omitempty"`
}

// RoutingSyncResultItem is one result from POST /v1/imports/sync (routing-service).
type RoutingSyncResultItem struct {
	Source        string `json:"source"`
	DatasetID     string `json:"dataset_id"`
	Status        string `json:"status"`
	RowsPersisted int64  `json:"rows_persisted"`
	DurationMs    int64  `json:"duration_ms"`
	Error         string `json:"error,omitempty"`
}

// RoutingSyncResponse is the response of routing POST /v1/imports/sync.
type RoutingSyncResponse struct {
	Results []RoutingSyncResultItem `json:"results"`
}

// RoutingSync calls POST /v1/imports/sync (routing-service).
func (c *Client) RoutingSync(ctx context.Context) (*RoutingSyncResponse, error) {
	var out RoutingSyncResponse
	if err := c.post(ctx, c.routing, "/v1/imports/sync", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RoutingByIP calls GET /v1/asn/by-ip/{ip}.
func (c *Client) RoutingByIP(ctx context.Context, ip, datasetID string) (*RoutingByIPResponse, error) {
	if ip == "" {
		return nil, fmt.Errorf("ip required")
	}
	path := "/v1/asn/by-ip/" + url.PathEscape(ip)
	if datasetID != "" {
		path += "?dataset_id=" + url.QueryEscape(datasetID)
	}
	var out RoutingByIPResponse
	if err := c.get(ctx, c.routing, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RoutingASNMeta calls GET /v1/asn/{asn}.
func (c *Client) RoutingASNMeta(ctx context.Context, asn int64) (*RoutingASNMetaResponse, error) {
	path := "/v1/asn/" + strconv.FormatInt(asn, 10)
	var out RoutingASNMetaResponse
	if err := c.get(ctx, c.routing, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RoutingASNPrefixes calls GET /v1/asn/prefixes/{asn}.
func (c *Client) RoutingASNPrefixes(ctx context.Context, asn int64, datasetID string, limit, offset int) (*RoutingASNPrefixesResponse, error) {
	path := "/v1/asn/prefixes/" + strconv.FormatInt(asn, 10)
	var q url.Values
	if datasetID != "" {
		q = url.Values{}
		q.Set("dataset_id", datasetID)
	}
	if limit > 0 {
		if q == nil {
			q = url.Values{}
		}
		q.Set("limit", strconv.Itoa(limit))
		q.Set("offset", strconv.Itoa(offset))
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out RoutingASNPrefixesResponse
	if err := c.get(ctx, c.routing, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
