package inventoryclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/execution-service/domain"
)

// PortDiscoveryResult is the worker result_summary shape (observations with ip, port, status).
type PortDiscoveryResult struct {
	Observations []struct {
		IP     string `json:"ip"`
		Port   int    `json:"port"`
		Status string `json:"status"`
	} `json:"observations"`
	Error string `json:"error,omitempty"`
}

// JobCompletedPayload is the explicit contract sent to inventory-service (not result_summary).
type JobCompletedPayload struct {
	ExecutionID             string    `json:"execution_id"`
	JobID                   string    `json:"job_id"`
	RunID                   string    `json:"run_id"`
	CampaignID              string    `json:"campaign_id"`
	TargetID                string    `json:"target_id"`
	TargetMaterializationID string    `json:"target_materialization_id"`
	ScanProfileSlug         string    `json:"scan_profile_slug"`
	ObservedAt               time.Time `json:"observed_at"`
	Observations             []struct {
		IP     string `json:"ip"`
		Port   int    `json:"port"`
		Status string `json:"status"`
	} `json:"observations"`
}

// Store is the minimal store interface needed to build the ingest payload.
type Store interface {
	GetJobByID(uuid.UUID) (*domain.ExecutionJob, error)
	GetExecutionByID(uuid.UUID) (*domain.Execution, error)
}

// Client calls inventory-service to notify job completion.
// Delivery is best-effort, not guaranteed: fire-and-forget goroutine with limited retries.
// If execution-service crashes after JobComplete but before or during the HTTP call, the
// ingest can be lost; no outbox or queue in v1. Do not rely on this for strong consistency.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Store      Store
	MaxRetries int
}

// NewClient returns a client that POSTs to baseURL (e.g. http://inventory-service:8086). If baseURL is empty, NotifyJobCompleted is a no-op.
func NewClient(baseURL string, store Store) *Client {
	if baseURL == "" {
		return &Client{Store: store}
	}
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		Store:      store,
		MaxRetries: 3,
	}
}

// NotifyJobCompleted builds the explicit payload from job + execution + result_summary and POSTs to inventory-service. Does not block; log and retry on failure.
func (c *Client) NotifyJobCompleted(ctx context.Context, jobID uuid.UUID, resultSummary json.RawMessage) {
	if c.BaseURL == "" {
		return
	}
	go func() {
		// Use background context for the async work so we're not tied to the request lifecycle.
		bg := context.Background()
		if err := c.notify(bg, jobID, resultSummary); err != nil {
			log.Printf("execution-service: inventory notify job_id=%s: %v", jobID, err)
		}
	}()
}

func (c *Client) notify(ctx context.Context, jobID uuid.UUID, resultSummary json.RawMessage) error {
	job, err := c.Store.GetJobByID(jobID)
	if err != nil || job == nil {
		return err
	}
	exec, err := c.Store.GetExecutionByID(job.ExecutionID)
	if err != nil || exec == nil {
		return err
	}
	var result PortDiscoveryResult
	if len(resultSummary) > 0 {
		if err := json.Unmarshal(resultSummary, &result); err != nil {
			return err
		}
	}
	observedAt := time.Now()
	if job.CompletedAt != nil {
		observedAt = *job.CompletedAt
	}
	payload := JobCompletedPayload{
		ExecutionID:             job.ExecutionID.String(),
		JobID:                   job.ID.String(),
		RunID:                   exec.RunID.String(),
		CampaignID:              exec.CampaignID.String(),
		TargetID:                exec.TargetID.String(),
		TargetMaterializationID: exec.TargetMaterializationID.String(),
		ScanProfileSlug:         exec.ScanProfileSlug,
		ObservedAt:              observedAt,
		Observations:            result.Observations,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := c.BaseURL + "/v1/ingest/job-completed"
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("execution-service: inventory ingest sent job_id=%s", jobID)
			return nil
		}
		if resp.StatusCode == 409 {
			log.Printf("execution-service: inventory ingest job_id=%s already ingested (409)", jobID)
			return nil
		}
		lastErr = fmt.Errorf("inventory returned %d", resp.StatusCode)
	}
	return lastErr
}
