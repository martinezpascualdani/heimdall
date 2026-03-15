package client

import (
	"context"
	"fmt"
	"net/url"
)

// Scan profile
type ScanProfileResponse struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Slug        string      `json:"slug"`
	Description string      `json:"description"`
	Config      interface{} `json:"config"`
	CreatedAt   string      `json:"created_at"`
	UpdatedAt   string      `json:"updated_at"`
}

type ScanProfileListResponse struct {
	Items   []ScanProfileResponse `json:"items"`
	Count   int                   `json:"count"`
	Total   int                   `json:"total"`
	Limit   int                   `json:"limit"`
	Offset  int                   `json:"offset"`
	HasMore bool                  `json:"has_more"`
}

type ScanProfileCreateInput struct {
	Name        string      `json:"name"`
	Slug        string      `json:"slug"`
	Description string      `json:"description,omitempty"`
	Config      interface{} `json:"config,omitempty"`
}

// Campaign
type CampaignResponse struct {
	ID                    string      `json:"id"`
	Name                  string      `json:"name"`
	Description           string      `json:"description"`
	Active                bool        `json:"active"`
	TargetID              string      `json:"target_id"`
	ScanProfileID         string      `json:"scan_profile_id"`
	ScheduleType          string      `json:"schedule_type"`
	ScheduleConfig        interface{} `json:"schedule_config"`
	MaterializationPolicy string      `json:"materialization_policy"`
	NextRunAt             string      `json:"next_run_at,omitempty"`
	RunOnceDone           bool        `json:"run_once_done"`
	ConcurrencyPolicy     string      `json:"concurrency_policy"`
	CreatedAt             string      `json:"created_at"`
	UpdatedAt             string      `json:"updated_at"`
}

type CampaignListResponse struct {
	Items   []CampaignResponse `json:"items"`
	Count   int                `json:"count"`
	Total   int                `json:"total"`
	Limit   int                `json:"limit"`
	Offset  int                `json:"offset"`
	HasMore bool               `json:"has_more"`
}

type CampaignCreateInput struct {
	Name                 string      `json:"name"`
	Description          string      `json:"description,omitempty"`
	TargetID             string      `json:"target_id"`
	ScanProfileID        string      `json:"scan_profile_id"`
	ScheduleType         string      `json:"schedule_type"`
	ScheduleConfig       interface{} `json:"schedule_config,omitempty"`
	MaterializationPolicy string      `json:"materialization_policy"`
	NextRunAt            string      `json:"next_run_at,omitempty"`
	ConcurrencyPolicy    string      `json:"concurrency_policy,omitempty"`
}

// Run
type CampaignRunResponse struct {
	ID                        string      `json:"id"`
	CampaignID                string      `json:"campaign_id"`
	TargetID                  string      `json:"target_id"`
	TargetMaterializationID   string      `json:"target_materialization_id"`
	ScanProfileID             string      `json:"scan_profile_id"`
	ScanProfileSlug          string      `json:"scan_profile_slug"`
	ScanProfileConfigSnapshot interface{} `json:"scan_profile_config_snapshot"`
	Status                    string      `json:"status"`
	CreatedAt                 string      `json:"created_at"`
	StartedAt                 string      `json:"started_at,omitempty"`
	CompletedAt               string      `json:"completed_at,omitempty"`
	DispatchedAt              string      `json:"dispatched_at,omitempty"`
	DispatchRef               string      `json:"dispatch_ref"`
	ErrorMessage              string      `json:"error_message"`
	Stats                     interface{} `json:"stats"`
}

type CampaignRunListResponse struct {
	Items   []CampaignRunResponse `json:"items"`
	Count   int                   `json:"count"`
	Total   int                   `json:"total"`
	Limit   int                   `json:"limit"`
	Offset  int                   `json:"offset"`
	HasMore bool                  `json:"has_more"`
}

// Scan profiles
func (c *Client) ScanProfileList(ctx context.Context, limit, offset int) (*ScanProfileListResponse, error) {
	path := "/v1/scan-profiles?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit)) + "&offset=" + url.QueryEscape(fmt.Sprintf("%d", offset))
	var out ScanProfileListResponse
	if err := c.get(ctx, c.campaign, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ScanProfileGet(ctx context.Context, id string) (*ScanProfileResponse, error) {
	path := "/v1/scan-profiles/" + url.PathEscape(id)
	var out ScanProfileResponse
	if err := c.get(ctx, c.campaign, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ScanProfileCreate(ctx context.Context, in *ScanProfileCreateInput) (*ScanProfileResponse, error) {
	var out ScanProfileResponse
	if err := c.postBody(ctx, c.campaign, "/v1/scan-profiles", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ScanProfileUpdate(ctx context.Context, id string, in *ScanProfileCreateInput) (*ScanProfileResponse, error) {
	path := "/v1/scan-profiles/" + url.PathEscape(id)
	var out ScanProfileResponse
	if err := c.put(ctx, c.campaign, path, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ScanProfileDelete(ctx context.Context, id string) error {
	path := "/v1/scan-profiles/" + url.PathEscape(id)
	return c.delete(ctx, c.campaign, path)
}

// Campaigns
func (c *Client) CampaignList(ctx context.Context, includeInactive bool, limit, offset int) (*CampaignListResponse, error) {
	path := "/v1/campaigns?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit)) + "&offset=" + url.QueryEscape(fmt.Sprintf("%d", offset))
	if includeInactive {
		path += "&include_inactive=true"
	}
	var out CampaignListResponse
	if err := c.get(ctx, c.campaign, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CampaignGet(ctx context.Context, id string) (*CampaignResponse, error) {
	path := "/v1/campaigns/" + url.PathEscape(id)
	var out CampaignResponse
	if err := c.get(ctx, c.campaign, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CampaignCreate(ctx context.Context, in *CampaignCreateInput) (*CampaignResponse, error) {
	var out CampaignResponse
	if err := c.postBody(ctx, c.campaign, "/v1/campaigns", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CampaignUpdate(ctx context.Context, id string, in *CampaignCreateInput) (*CampaignResponse, error) {
	path := "/v1/campaigns/" + url.PathEscape(id)
	var out CampaignResponse
	if err := c.put(ctx, c.campaign, path, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CampaignDelete(ctx context.Context, id string) error {
	path := "/v1/campaigns/" + url.PathEscape(id)
	return c.delete(ctx, c.campaign, path)
}

func (c *Client) CampaignLaunch(ctx context.Context, campaignID string) (*CampaignRunResponse, error) {
	path := "/v1/campaigns/" + url.PathEscape(campaignID) + "/launch"
	var out CampaignRunResponse
	if err := c.postBody(ctx, c.campaign, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Runs
func (c *Client) CampaignRunList(ctx context.Context, campaignID string, limit, offset int) (*CampaignRunListResponse, error) {
	path := "/v1/campaigns/" + url.PathEscape(campaignID) + "/runs?limit=" + url.QueryEscape(fmt.Sprintf("%d", limit)) + "&offset=" + url.QueryEscape(fmt.Sprintf("%d", offset))
	var out CampaignRunListResponse
	if err := c.get(ctx, c.campaign, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RunGet(ctx context.Context, runID string) (*CampaignRunResponse, error) {
	path := "/v1/runs/" + url.PathEscape(runID)
	var out CampaignRunResponse
	if err := c.get(ctx, c.campaign, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) RunCancel(ctx context.Context, runID string) (*CampaignRunResponse, error) {
	path := "/v1/runs/" + url.PathEscape(runID) + "/cancel"
	var out CampaignRunResponse
	if err := c.postBody(ctx, c.campaign, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
