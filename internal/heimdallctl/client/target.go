package client

import (
	"context"
	"fmt"
	"net/url"
)

// TargetRuleInput is a rule for create/update target request.
type TargetRuleInput struct {
	Kind          string `json:"kind"`
	SelectorType  string `json:"selector_type"`
	SelectorValue string `json:"selector_value"`
	AddressFamily string `json:"address_family,omitempty"`
	RuleOrder     int    `json:"rule_order"`
}

// TargetCreateInput is the body for POST /v1/targets.
type TargetCreateInput struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Rules       []TargetRuleInput `json:"rules,omitempty"`
}

// TargetRuleResp is a rule in API response.
type TargetRuleResp struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	SelectorType  string `json:"selector_type"`
	SelectorValue string `json:"selector_value"`
	AddressFamily string `json:"address_family,omitempty"`
	RuleOrder     int    `json:"rule_order"`
}

// TargetResponse is target with rules (GET/POST/PUT response).
type TargetResponse struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Active      bool            `json:"active"`
	Tags        []string        `json:"tags,omitempty"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	Rules       []TargetRuleResp `json:"rules"`
}

// TargetListResponse is GET /v1/targets response.
type TargetListResponse struct {
	Items []TargetResponse `json:"items"`
	Count int               `json:"count"`
}

// TargetMaterializeResponse is POST .../materialize response.
type TargetMaterializeResponse struct {
	MaterializationID  string `json:"materialization_id"`
	Status             string `json:"status"`
	TotalPrefixCount   int    `json:"total_prefix_count"`
	MaterializedAt     string `json:"materialized_at"`
}

// TargetMaterializationItem is one snapshot in list.
type TargetMaterializationItem struct {
	ID                 string `json:"id"`
	TargetID           string `json:"target_id"`
	MaterializedAt     string `json:"materialized_at"`
	TotalPrefixCount   int    `json:"total_prefix_count"`
	Status             string `json:"status"`
	ErrorMessage       string `json:"error_message,omitempty"`
}

// TargetMaterializationsResponse is GET .../materializations response.
type TargetMaterializationsResponse struct {
	Items []TargetMaterializationItem `json:"items"`
	Count int                         `json:"count"`
}

// TargetPrefixesResponse is GET .../prefixes response.
type TargetPrefixesResponse struct {
	MaterializationID string   `json:"materialization_id"`
	Count             int      `json:"count"`
	Total             int      `json:"total"`
	Limit             int      `json:"limit"`
	Offset            int      `json:"offset"`
	HasMore           bool     `json:"has_more"`
	Items             []string `json:"items"`
}

// TargetDiffResponse is GET .../diff response.
type TargetDiffResponse struct {
	FromMaterializationID string   `json:"from_materialization_id"`
	ToMaterializationID  string   `json:"to_materialization_id"`
	FromMaterializedAt   string   `json:"from_materialized_at"`
	ToMaterializedAt    string   `json:"to_materialized_at"`
	AddedCount           int      `json:"added_count"`
	RemovedCount         int      `json:"removed_count"`
	Added                []string `json:"added"`
	Removed              []string `json:"removed"`
}

// TargetList calls GET /v1/targets.
func (c *Client) TargetList(ctx context.Context, includeInactive bool, limit, offset int) (*TargetListResponse, error) {
	path := "/v1/targets?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit)) + "&offset=" + url.QueryEscape(fmt.Sprintf("%d", offset))
	if includeInactive {
		path += "&include_inactive=true"
	}
	var out TargetListResponse
	if err := c.get(ctx, c.target, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TargetGet calls GET /v1/targets/{id}.
func (c *Client) TargetGet(ctx context.Context, id string) (*TargetResponse, error) {
	path := "/v1/targets/" + url.PathEscape(id)
	var out TargetResponse
	if err := c.get(ctx, c.target, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TargetCreate calls POST /v1/targets.
func (c *Client) TargetCreate(ctx context.Context, in *TargetCreateInput) (*TargetResponse, error) {
	var out TargetResponse
	if err := c.postBody(ctx, c.target, "/v1/targets", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TargetUpdate calls PUT /v1/targets/{id}.
func (c *Client) TargetUpdate(ctx context.Context, id string, in *TargetCreateInput) (*TargetResponse, error) {
	path := "/v1/targets/" + url.PathEscape(id)
	var out TargetResponse
	if err := c.put(ctx, c.target, path, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TargetDelete calls DELETE /v1/targets/{id}.
func (c *Client) TargetDelete(ctx context.Context, id string) error {
	path := "/v1/targets/" + url.PathEscape(id)
	return c.delete(ctx, c.target, path)
}

// TargetMaterialize calls POST /v1/targets/{id}/materialize.
func (c *Client) TargetMaterialize(ctx context.Context, id string) (*TargetMaterializeResponse, error) {
	path := "/v1/targets/" + url.PathEscape(id) + "/materialize"
	var out TargetMaterializeResponse
	if err := c.postBody(ctx, c.target, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TargetMaterializations calls GET /v1/targets/{id}/materializations.
func (c *Client) TargetMaterializations(ctx context.Context, targetID string, limit, offset int) (*TargetMaterializationsResponse, error) {
	path := "/v1/targets/" + url.PathEscape(targetID) + "/materializations?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit)) + "&offset=" + url.QueryEscape(fmt.Sprintf("%d", offset))
	var out TargetMaterializationsResponse
	if err := c.get(ctx, c.target, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TargetMaterializationGet calls GET /v1/targets/{id}/materializations/{mid}.
func (c *Client) TargetMaterializationGet(ctx context.Context, targetID, mid string) (*TargetMaterializationItem, error) {
	path := "/v1/targets/" + url.PathEscape(targetID) + "/materializations/" + url.PathEscape(mid)
	var out TargetMaterializationItem
	if err := c.get(ctx, c.target, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TargetPrefixes calls GET /v1/targets/{id}/materializations/{mid}/prefixes.
func (c *Client) TargetPrefixes(ctx context.Context, targetID, mid string, limit, offset int) (*TargetPrefixesResponse, error) {
	path := "/v1/targets/" + url.PathEscape(targetID) + "/materializations/" + url.PathEscape(mid) + "/prefixes?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit)) + "&offset=" + url.QueryEscape(fmt.Sprintf("%d", offset))
	var out TargetPrefixesResponse
	if err := c.get(ctx, c.target, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TargetDiff calls GET /v1/targets/{id}/materializations/diff?from=&to=.
func (c *Client) TargetDiff(ctx context.Context, targetID, fromMid, toMid string) (*TargetDiffResponse, error) {
	path := "/v1/targets/" + url.PathEscape(targetID) + "/materializations/diff?from=" + url.QueryEscape(fromMid) + "&to=" + url.QueryEscape(toMid)
	var out TargetDiffResponse
	if err := c.get(ctx, c.target, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
