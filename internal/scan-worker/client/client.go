package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Client is the HTTP client for execution-service (register, heartbeat, claim, complete, fail, renew).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a client. baseURL should not have trailing slash.
func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

// RegisterRequest is sent to POST /v1/workers.
type RegisterRequest struct {
	Name           string   `json:"name"`
	Region         string   `json:"region"`
	Version        string   `json:"version"`
	Capabilities   []string `json:"capabilities"`
	MaxConcurrency int      `json:"max_concurrency"`
}

// RegisterResponse is the response from register.
type RegisterResponse struct {
	WorkerID       uuid.UUID `json:"worker_id"`
	Name           string    `json:"name"`
	Region         string    `json:"region"`
	Version        string    `json:"version"`
	Capabilities   []string  `json:"capabilities"`
	MaxConcurrency int       `json:"max_concurrency"`
	Status         string    `json:"status"`
}

// Register registers the worker and returns the worker ID.
func (c *Client) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/workers", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("register: %s", resp.Status)
	}
	var out RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Heartbeat calls PATCH /v1/workers/{id}. Optional body can update capabilities/max_concurrency.
func (c *Client) Heartbeat(ctx context.Context, workerID uuid.UUID, body interface{}) error {
	var bodyReader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader([]byte("{}"))
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.BaseURL+"/v1/workers/"+workerID.String(), bodyReader)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat: %s", resp.Status)
	}
	return nil
}

// ClaimRequest is sent to POST /v1/jobs/claim.
type ClaimRequest struct {
	WorkerID     string   `json:"worker_id"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// ClaimedJob is a job returned from claim.
type ClaimedJob struct {
	ID             uuid.UUID       `json:"id"`
	ExecutionID    uuid.UUID       `json:"execution_id"`
	Payload        json.RawMessage `json:"payload"`
	Status         string          `json:"status"`
	LeaseID        string          `json:"lease_id"`
	LeaseExpiresAt time.Time       `json:"lease_expires_at"`
	Attempt        int             `json:"attempt"`
	MaxAttempts    int             `json:"max_attempts"`
}

// ClaimResponse is the response from claim (job may be null).
type ClaimResponse struct {
	ID             *uuid.UUID     `json:"id,omitempty"`
	ExecutionID    *uuid.UUID     `json:"execution_id,omitempty"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	Status         string         `json:"status,omitempty"`
	LeaseID        string         `json:"lease_id,omitempty"`
	LeaseExpiresAt *time.Time     `json:"lease_expires_at,omitempty"`
	Attempt        int            `json:"attempt,omitempty"`
	MaxAttempts    int            `json:"max_attempts,omitempty"`
	Job            *ClaimedJob    `json:"job,omitempty"`
}

// Claim requests a job. Returns (nil, nil) when no job available.
func (c *Client) Claim(ctx context.Context, workerID uuid.UUID, capabilities []string) (*ClaimedJob, error) {
	req := ClaimRequest{WorkerID: workerID.String(), Capabilities: capabilities}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/jobs/claim", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claim: %s", resp.Status)
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	// API returns {"job": null} when no job, or {"id": "...", "lease_id": "...", ...} when job present
	if raw["job"] != nil {
		var nullJob *ClaimedJob
		_ = json.Unmarshal(raw["job"], &nullJob)
		return nullJob, nil
	}
	if raw["id"] == nil {
		return nil, nil
	}
	var j ClaimedJob
	_ = json.Unmarshal(raw["id"], &j.ID)
	if v, ok := raw["lease_id"]; ok {
		_ = json.Unmarshal(v, &j.LeaseID)
	}
	if v, ok := raw["lease_expires_at"]; ok {
		_ = json.Unmarshal(v, &j.LeaseExpiresAt)
	}
	if v, ok := raw["payload"]; ok {
		j.Payload = v
	}
	if v, ok := raw["execution_id"]; ok {
		_ = json.Unmarshal(v, &j.ExecutionID)
	}
	if v, ok := raw["status"]; ok {
		_ = json.Unmarshal(v, &j.Status)
	}
	if v, ok := raw["attempt"]; ok {
		_ = json.Unmarshal(v, &j.Attempt)
	}
	if v, ok := raw["max_attempts"]; ok {
		_ = json.Unmarshal(v, &j.MaxAttempts)
	}
	return &j, nil
}

// Complete reports job completed.
func (c *Client) Complete(ctx context.Context, jobID, workerID uuid.UUID, leaseID string, resultSummary json.RawMessage) error {
	body, _ := json.Marshal(map[string]interface{}{
		"worker_id":      workerID.String(),
		"lease_id":       leaseID,
		"result_summary": resultSummary,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/jobs/"+jobID.String()+"/complete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("complete: %s", resp.Status)
	}
	return nil
}

// Fail reports job failed.
func (c *Client) Fail(ctx context.Context, jobID, workerID uuid.UUID, leaseID string, errorMessage string) error {
	body, _ := json.Marshal(map[string]interface{}{
		"worker_id":     workerID.String(),
		"lease_id":      leaseID,
		"error_message": errorMessage,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/jobs/"+jobID.String()+"/fail", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fail: %s", resp.Status)
	}
	return nil
}

// Renew extends the job lease.
func (c *Client) Renew(ctx context.Context, jobID, workerID uuid.UUID, leaseID string) error {
	body, _ := json.Marshal(map[string]string{
		"worker_id": workerID.String(),
		"lease_id":  leaseID,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/jobs/"+jobID.String()+"/renew", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("renew: %s", resp.Status)
	}
	return nil
}
