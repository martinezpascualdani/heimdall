package targetclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Client calls target-service for list materializations, materialize, and optional target validation.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient returns a client. baseURL should not have trailing slash.
func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 10 * time.Minute // materialize can be long
	}
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

// MaterializationItem is one item from GET .../materializations.
type MaterializationItem struct {
	ID                 uuid.UUID `json:"id"`
	TargetID           uuid.UUID `json:"target_id"`
	MaterializedAt     string    `json:"materialized_at"`
	TotalPrefixCount   int       `json:"total_prefix_count"`
	Status             string    `json:"status"`
}

// ListMaterializationsResponse is the response from GET /v1/targets/{id}/materializations.
type ListMaterializationsResponse struct {
	Items []MaterializationItem `json:"items"`
	Count int                   `json:"count"`
}

// ListMaterializations calls GET /v1/targets/{id}/materializations?limit=&offset=.
// Used for use_latest: call with limit=1, offset=0 to get the most recent.
func (c *Client) ListMaterializations(ctx context.Context, targetID uuid.UUID, limit, offset int) (*ListMaterializationsResponse, error) {
	u := fmt.Sprintf("%s/v1/targets/%s/materializations?limit=%d&offset=%d", c.BaseURL, targetID.String(), limit, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("target-service list materializations: %s", resp.Status)
	}
	var out ListMaterializationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MaterializeResponse is the response from POST /v1/targets/{id}/materialize.
type MaterializeResponse struct {
	MaterializationID  uuid.UUID `json:"materialization_id"`
	Status             string    `json:"status"`
	TotalPrefixCount   int       `json:"total_prefix_count"`
	MaterializedAt     string    `json:"materialized_at,omitempty"`
}

// Materialize calls POST /v1/targets/{id}/materialize. Returns materialization_id and total_prefix_count.
func (c *Client) Materialize(ctx context.Context, targetID uuid.UUID) (*MaterializeResponse, error) {
	u := fmt.Sprintf("%s/v1/targets/%s/materialize", c.BaseURL, targetID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("target-service materialize: %s", resp.Status)
	}
	var out MaterializeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TargetExists checks that the target exists (for create/update campaign validation).
// Uses GET /v1/targets/{id}; 404 or error means not found.
func (c *Client) TargetExists(ctx context.Context, targetID uuid.UUID) (bool, error) {
	u := fmt.Sprintf("%s/v1/targets/%s", c.BaseURL, targetID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("target-service get target: %s", resp.Status)
}
