package targetclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Client calls target-service for listing prefixes of a materialization (paginated).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient returns a client. baseURL should not have trailing slash.
func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: timeout},
	}
}

// ListPrefixesResponse is the response from GET /v1/targets/{id}/materializations/{mid}/prefixes.
type ListPrefixesResponse struct {
	MaterializationID uuid.UUID `json:"materialization_id"`
	Count             int      `json:"count"`
	Total             int      `json:"total"`
	Limit             int      `json:"limit"`
	Offset            int      `json:"offset"`
	HasMore           bool     `json:"has_more"`
	Items             []string `json:"items"`
}

// ListPrefixesPage calls GET /v1/targets/{targetID}/materializations/{materializationID}/prefixes?limit=&offset=.
func (c *Client) ListPrefixesPage(ctx context.Context, targetID, materializationID uuid.UUID, limit, offset int) (*ListPrefixesResponse, error) {
	u := fmt.Sprintf("%s/v1/targets/%s/materializations/%s/prefixes?limit=%d&offset=%d",
		c.BaseURL, targetID.String(), materializationID.String(), limit, offset)
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
		return nil, fmt.Errorf("target-service list prefixes: %s", resp.Status)
	}
	var out ListPrefixesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAllPrefixes fetches all prefixes for a materialization by paginating until has_more is false.
// PageSize is the limit per request. If maxPrefixes > 0, stops after collecting that many (safety cap).
func (c *Client) GetAllPrefixes(ctx context.Context, targetID, materializationID uuid.UUID, pageSize int, maxPrefixes int) ([]string, error) {
	if pageSize <= 0 {
		pageSize = 1000
	}
	var all []string
	offset := 0
	for {
		page, err := c.ListPrefixesPage(ctx, targetID, materializationID, pageSize, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if !page.HasMore {
			break
		}
		offset += len(page.Items)
		if maxPrefixes > 0 && len(all) >= maxPrefixes {
			break
		}
	}
	return all, nil
}
