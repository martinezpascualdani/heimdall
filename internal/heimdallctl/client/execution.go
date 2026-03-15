package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// ExecutionItem is one execution (from list or get).
type ExecutionItem struct {
	ID                       string          `json:"id"`
	RunID                    string          `json:"run_id"`
	CampaignID               string          `json:"campaign_id"`
	TargetID                 string          `json:"target_id"`
	TargetMaterializationID  string          `json:"target_materialization_id"`
	ScanProfileSlug          string          `json:"scan_profile_slug"`
	ScanProfileConfig        json.RawMessage `json:"scan_profile_config,omitempty"`
	Status                   string          `json:"status"`
	TotalJobs                int             `json:"total_jobs"`
	CompletedJobs            int             `json:"completed_jobs"`
	FailedJobs               int             `json:"failed_jobs"`
	CreatedAt                string          `json:"created_at"`
	UpdatedAt                string          `json:"updated_at"`
	CompletedAt              *string         `json:"completed_at,omitempty"`
	ErrorSummary             string          `json:"error_summary,omitempty"`
}

// ExecutionListResponse is GET /v1/executions response.
type ExecutionListResponse struct {
	Items  []ExecutionItem `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// ExecutionJobItem is one job (from list execution jobs or list worker jobs).
type ExecutionJobItem struct {
	ID               string          `json:"id"`
	ExecutionID      string          `json:"execution_id"`
	Payload          json.RawMessage `json:"payload"`
	Status           string          `json:"status"`
	AssignedWorkerID *string         `json:"assigned_worker_id,omitempty"`
	LeaseExpiresAt   *string         `json:"lease_expires_at,omitempty"`
	LeaseID          string          `json:"lease_id,omitempty"`
	Attempt          int             `json:"attempt"`
	MaxAttempts      int             `json:"max_attempts"`
	ErrorMessage     string          `json:"error_message,omitempty"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
	StartedAt        *string         `json:"started_at,omitempty"`
	CompletedAt      *string         `json:"completed_at,omitempty"`
	ResultSummary    json.RawMessage `json:"result_summary,omitempty"`
}

// ExecutionJobsResponse is GET /v1/executions/{id}/jobs response.
type ExecutionJobsResponse struct {
	Items  []ExecutionJobItem `json:"items"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

// WorkerItem is one worker (from list or get).
type WorkerItem struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Region            string   `json:"region"`
	Version           string   `json:"version"`
	Capabilities      []string `json:"capabilities"`
	Status            string   `json:"status"`
	LastHeartbeatAt   *string  `json:"last_heartbeat_at,omitempty"`
	MaxConcurrency    int      `json:"max_concurrency"`
	CurrentLoad       int      `json:"current_load"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

// WorkerListResponse is GET /v1/workers response.
type WorkerListResponse struct {
	Items  []WorkerItem `json:"items"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// WorkerJobsResponse is GET /v1/workers/{id}/jobs response (jobs assigned to this worker).
type WorkerJobsResponse struct {
	Items []ExecutionJobItem `json:"items"`
	Total int                `json:"total"`
	Limit int                `json:"limit"`
}

// ExecutionList calls GET /v1/executions with optional run_id, campaign_id, status, limit, offset.
func (c *Client) ExecutionList(ctx context.Context, runID, campaignID, status string, limit, offset int) (*ExecutionListResponse, error) {
	path := "/v1/executions?"
	q := url.Values{}
	if runID != "" {
		q.Set("run_id", runID)
	}
	if campaignID != "" {
		q.Set("campaign_id", campaignID)
	}
	if status != "" {
		q.Set("status", status)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", offset))
	}
	path += q.Encode()
	var out ExecutionListResponse
	if err := c.get(ctx, c.execution, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExecutionGet calls GET /v1/executions/{id}.
func (c *Client) ExecutionGet(ctx context.Context, id string) (*ExecutionItem, error) {
	if id == "" {
		return nil, fmt.Errorf("execution id required")
	}
	path := "/v1/executions/" + url.PathEscape(id)
	var out ExecutionItem
	if err := c.get(ctx, c.execution, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExecutionListJobs calls GET /v1/executions/{id}/jobs.
func (c *Client) ExecutionListJobs(ctx context.Context, executionID string, limit, offset int) (*ExecutionJobsResponse, error) {
	if executionID == "" {
		return nil, fmt.Errorf("execution id required")
	}
	path := "/v1/executions/" + url.PathEscape(executionID) + "/jobs?"
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", offset))
	}
	path += q.Encode()
	var out ExecutionJobsResponse
	if err := c.get(ctx, c.execution, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExecutionRequeue calls POST /v1/executions/{id}/requeue.
func (c *Client) ExecutionRequeue(ctx context.Context, id string) (requeued int, err error) {
	if id == "" {
		return 0, fmt.Errorf("execution id required")
	}
	path := "/v1/executions/" + url.PathEscape(id) + "/requeue"
	var out struct {
		Requeued int `json:"requeued"`
	}
	if err := c.post(ctx, c.execution, path, &out); err != nil {
		return 0, err
	}
	return out.Requeued, nil
}

// ExecutionCancel calls POST /v1/executions/{id}/cancel.
func (c *Client) ExecutionCancel(ctx context.Context, id string) (canceled int, err error) {
	if id == "" {
		return 0, fmt.Errorf("execution id required")
	}
	path := "/v1/executions/" + url.PathEscape(id) + "/cancel"
	var out struct {
		Canceled int    `json:"canceled"`
		Status   string `json:"status"`
	}
	if err := c.post(ctx, c.execution, path, &out); err != nil {
		return 0, err
	}
	return out.Canceled, nil
}

// WorkerList calls GET /v1/workers with optional status, limit, offset.
func (c *Client) WorkerList(ctx context.Context, status string, limit, offset int) (*WorkerListResponse, error) {
	path := "/v1/workers?"
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", offset))
	}
	path += q.Encode()
	var out WorkerListResponse
	if err := c.get(ctx, c.execution, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WorkerGet calls GET /v1/workers/{id}.
func (c *Client) WorkerGet(ctx context.Context, id string) (*WorkerItem, error) {
	if id == "" {
		return nil, fmt.Errorf("worker id required")
	}
	path := "/v1/workers/" + url.PathEscape(id)
	var out WorkerItem
	if err := c.get(ctx, c.execution, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WorkerListJobs calls GET /v1/workers/{id}/jobs (jobs currently assigned/running on this worker).
func (c *Client) WorkerListJobs(ctx context.Context, workerID string, limit int) (*WorkerJobsResponse, error) {
	if workerID == "" {
		return nil, fmt.Errorf("worker id required")
	}
	path := "/v1/workers/" + url.PathEscape(workerID) + "/jobs?"
	if limit > 0 {
		path += "limit=" + fmt.Sprintf("%d", limit)
	}
	if path[len(path)-1] == '?' {
		path = path[:len(path)-1]
	}
	var out WorkerJobsResponse
	if err := c.get(ctx, c.execution, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WorkerUpdateMaxConcurrency sends PATCH /v1/workers/{id} with max_concurrency (heartbeat endpoint updates DB).
func (c *Client) WorkerUpdateMaxConcurrency(ctx context.Context, workerID string, maxConcurrency int) error {
	if workerID == "" {
		return fmt.Errorf("worker id required")
	}
	if maxConcurrency <= 0 {
		return fmt.Errorf("max_concurrency must be >= 1")
	}
	path := "/v1/workers/" + url.PathEscape(workerID)
	body := struct {
		MaxConcurrency int `json:"max_concurrency"`
	}{MaxConcurrency: maxConcurrency}
	return c.patchBody(ctx, c.execution, path, body, nil)
}
