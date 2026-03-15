package materializer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ScopeBlocksItem is one block from GET /v1/scopes/country/{cc}/blocks (items[].normalized).
type ScopeBlocksItem struct {
	Normalized []string `json:"normalized"`
}

// ScopeBlocksResponse is the response from scope-service blocks endpoint.
type ScopeBlocksResponse struct {
	Count        int               `json:"count"`
	Total        int               `json:"total"`
	Limit        int               `json:"limit"`
	Offset       int               `json:"offset"`
	HasMore      bool              `json:"has_more"`
	DatasetsUsed []struct {
		DatasetID string `json:"dataset_id"`
	} `json:"datasets_used"`
	Items []ScopeBlocksItem `json:"items"`
}

// RoutingPrefixItem is one prefix from GET /v1/asn/prefixes/{asn}.
type RoutingPrefixItem struct {
	Prefix string `json:"prefix"`
}

// RoutingPrefixesResponse is the response from routing-service ASN prefixes endpoint.
type RoutingPrefixesResponse struct {
	Items      []RoutingPrefixItem `json:"items"`
	Total      int                 `json:"total"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
	HasMore    bool                `json:"has_more"`
	DatasetID  string              `json:"dataset_id,omitempty"`
}

// ScopeClient fetches country blocks from scope-service.
type ScopeClient struct {
	BaseURL string
	Client  *http.Client
}

// RoutingClient fetches ASN prefixes from routing-service.
type RoutingClient struct {
	BaseURL string
	Client  *http.Client
}

// FetchCountryBlocks returns all normalized CIDRs for a country (paginates until done).
func (c *ScopeClient) FetchCountryBlocks(ctx context.Context, cc, addressFamily string) ([]string, []string, time.Time, error) {
	var all []string
	var datasetIDs []string
	limit := 5000
	offset := 0
	resolvedAt := time.Now()
	for {
		path := fmt.Sprintf("/v1/scopes/country/%s/blocks?limit=%d&offset=%d", url.PathEscape(cc), limit, offset)
		if addressFamily != "" && addressFamily != "all" {
			path += "&address_family=" + url.QueryEscape(addressFamily)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
		if err != nil {
			return nil, nil, resolvedAt, err
		}
		resp, err := c.Client.Do(req)
		if err != nil {
			return nil, nil, resolvedAt, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, nil, resolvedAt, fmt.Errorf("scope blocks: %s %d", cc, resp.StatusCode)
		}
		var data ScopeBlocksResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			resp.Body.Close()
			return nil, nil, resolvedAt, err
		}
		resp.Body.Close()
		for _, du := range data.DatasetsUsed {
			if du.DatasetID != "" {
				datasetIDs = append(datasetIDs, du.DatasetID)
			}
		}
		for _, it := range data.Items {
			for _, n := range it.Normalized {
				if n != "" {
					all = append(all, n)
				}
			}
		}
		if !data.HasMore || len(data.Items) == 0 {
			break
		}
		offset += len(data.Items)
	}
	return all, datasetIDs, resolvedAt, nil
}

// FetchASNPrefixes returns all prefixes for an ASN (paginates until done).
func (c *RoutingClient) FetchASNPrefixes(ctx context.Context, asn string, addressFamily string) ([]string, []string, time.Time, error) {
	var all []string
	var datasetIDs []string
	limit := 5000
	offset := 0
	resolvedAt := time.Now()
	for {
		path := fmt.Sprintf("/v1/asn/prefixes/%s?limit=%d&offset=%d", url.PathEscape(asn), limit, offset)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
		if err != nil {
			return nil, nil, resolvedAt, err
		}
		resp, err := c.Client.Do(req)
		if err != nil {
			return nil, nil, resolvedAt, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, nil, resolvedAt, fmt.Errorf("routing prefixes asn=%s: %d", asn, resp.StatusCode)
		}
		var data RoutingPrefixesResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			resp.Body.Close()
			return nil, nil, resolvedAt, err
		}
		resp.Body.Close()
		if data.DatasetID != "" {
			datasetIDs = append(datasetIDs, data.DatasetID)
		}
		for _, it := range data.Items {
			if it.Prefix != "" {
				all = append(all, it.Prefix)
			}
		}
		if !data.HasMore || len(data.Items) == 0 {
			break
		}
		offset += len(data.Items)
	}
	_ = addressFamily
	return all, datasetIDs, resolvedAt, nil
}

// FetchCountryBlocksWithDatasetID is like FetchCountryBlocks but with optional dataset_id (for snapshot ref).
func (c *ScopeClient) FetchCountryBlocksWithDatasetID(ctx context.Context, cc, datasetID, addressFamily string) ([]string, []string, time.Time, error) {
	var all []string
	var datasetIDs []string
	limit := 5000
	offset := 0
	resolvedAt := time.Now()
	for {
		path := fmt.Sprintf("/v1/scopes/country/%s/blocks?limit=%d&offset=%d", url.PathEscape(cc), limit, offset)
		if datasetID != "" {
			path += "&dataset_id=" + url.QueryEscape(datasetID)
		}
		if addressFamily != "" && addressFamily != "all" {
			path += "&address_family=" + url.QueryEscape(addressFamily)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
		if err != nil {
			return nil, nil, resolvedAt, err
		}
		resp, err := c.Client.Do(req)
		if err != nil {
			return nil, nil, resolvedAt, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, nil, resolvedAt, fmt.Errorf("scope blocks: %s %d", cc, resp.StatusCode)
		}
		var data ScopeBlocksResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			resp.Body.Close()
			return nil, nil, resolvedAt, err
		}
		resp.Body.Close()
		for _, it := range data.Items {
			for _, n := range it.Normalized {
				if n != "" {
					all = append(all, n)
				}
			}
		}
		if !data.HasMore || len(data.Items) == 0 {
			break
		}
		offset += len(data.Items)
	}
	if datasetID != "" {
		datasetIDs = append(datasetIDs, datasetID)
	}
	return all, datasetIDs, resolvedAt, nil
}

// FetchASNPrefixesWithDatasetID is like FetchASNPrefixes but with optional dataset_id.
func (c *RoutingClient) FetchASNPrefixesWithDatasetID(ctx context.Context, asn, datasetID string) ([]string, []string, time.Time, error) {
	all, ids, t, err := c.FetchASNPrefixes(ctx, asn, "")
	if datasetID != "" {
		ids = append(ids, datasetID)
	}
	return all, ids, t, err
}
