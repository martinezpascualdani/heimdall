package client

import (
	"context"
	"fmt"
	"net/url"
)

// DatasetVersion is a single dataset version (dataset-service API).
type DatasetVersion struct {
	ID            string `json:"id"`
	Registry      string `json:"registry,omitempty"`
	Serial        int64  `json:"serial,omitempty"`
	SourceType    string `json:"source_type"`
	Source        string `json:"source"`
	SourceVersion string `json:"source_version,omitempty"`
	StartDate     string `json:"start_date,omitempty"`
	EndDate       string `json:"end_date,omitempty"`
	RecordCount   int64  `json:"record_count"`
	Checksum      string `json:"checksum,omitempty"`
	State         string `json:"state"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	Error         string `json:"error,omitempty"`
}

// FetchResultSingle is the response of POST /v1/datasets/fetch (single registry/source).
type FetchResultSingle struct {
	Status    string `json:"status"`
	DatasetID string `json:"dataset_id"`
	Registry  string `json:"registry,omitempty"`
	Serial    int64  `json:"serial,omitempty"`
	State     string `json:"state,omitempty"`
	Error     string `json:"error,omitempty"`
}

// FetchResultAll is the response when fetching registry=all.
type FetchResultAll struct {
	Results []FetchResultSingle `json:"results"`
}

// DatasetListResponse is GET /v1/datasets response.
type DatasetListResponse struct {
	Datasets []DatasetVersion `json:"datasets"`
}

// fetchResponse is the union of single and batch fetch responses.
type fetchResponse struct {
	Results    []FetchResultSingle `json:"results"`
	Status     string              `json:"status"`
	DatasetID  string              `json:"dataset_id"`
	Registry   string              `json:"registry,omitempty"`
	Serial     int64               `json:"serial,omitempty"`
	State      string              `json:"state,omitempty"`
	Error      string              `json:"error,omitempty"`
}

// DatasetFetch calls POST /v1/datasets/fetch. registry: ripencc|arin|apnic|lacnic|afrinic|all; or use source for CAIDA.
// Returns FetchResultAll when response has "results" array, otherwise FetchResultSingle.
func (c *Client) DatasetFetch(ctx context.Context, registry, source string) (interface{}, error) {
	path := "/v1/datasets/fetch?"
	if source != "" {
		path += "source=" + url.QueryEscape(source)
	} else {
		if registry == "" {
			registry = "all"
		}
		path += "registry=" + url.QueryEscape(registry)
	}
	var raw fetchResponse
	if err := c.post(ctx, c.dataset, path, &raw); err != nil {
		return nil, err
	}
	if len(raw.Results) > 0 {
		return &FetchResultAll{Results: raw.Results}, nil
	}
	return &FetchResultSingle{
		Status:    raw.Status,
		DatasetID: raw.DatasetID,
		Registry:  raw.Registry,
		Serial:    raw.Serial,
		State:     raw.State,
		Error:     raw.Error,
	}, nil
}

// DatasetList calls GET /v1/datasets with optional source and source_type filters.
func (c *Client) DatasetList(ctx context.Context, source, sourceType string) (*DatasetListResponse, error) {
	path := "/v1/datasets"
	if source != "" || sourceType != "" {
		path += "?"
		if source != "" {
			path += "source=" + url.QueryEscape(source) + "&"
		}
		if sourceType != "" {
			path += "source_type=" + url.QueryEscape(sourceType)
		}
	}
	var out DatasetListResponse
	if err := c.get(ctx, c.dataset, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DatasetGet calls GET /v1/datasets/{id}.
func (c *Client) DatasetGet(ctx context.Context, id string) (*DatasetVersion, error) {
	if id == "" {
		return nil, fmt.Errorf("dataset id required")
	}
	path := "/v1/datasets/" + url.PathEscape(id)
	var out DatasetVersion
	if err := c.get(ctx, c.dataset, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
