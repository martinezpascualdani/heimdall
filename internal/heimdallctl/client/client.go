package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/config"
)

// Client calls Heimdall HTTP APIs. No business logic; only HTTP and JSON.
type Client struct {
	cfg      *config.Config
	http     *http.Client
	dataset  string
	scope    string
	routing  string
	target   string
	campaign string
}

// New builds a Client from config. Base URLs are trimmed of trailing slashes.
func New(cfg *config.Config) *Client {
	if cfg == nil {
		cfg = config.Load()
	}
	c := &Client{
		cfg:      cfg,
		dataset:  strings.TrimSuffix(cfg.DatasetURL, "/"),
		scope:    strings.TrimSuffix(cfg.ScopeURL, "/"),
		routing:  strings.TrimSuffix(cfg.RoutingURL, "/"),
		target:   strings.TrimSuffix(cfg.TargetURL, "/"),
		campaign: strings.TrimSuffix(cfg.CampaignURL, "/"),
	}
	c.http = &http.Client{Timeout: cfg.Timeout}
	return c
}

// WithTimeout returns a clone of the client with a custom timeout (e.g. for long syncs).
func (c *Client) WithTimeout(d time.Duration) *Client {
	out := *c
	out.http = &http.Client{Timeout: d}
	return &out
}

// apiError is returned when the API responds with status >= 400.
type apiError struct {
	StatusCode int
	Body       string
	Message    string
}

func (e *apiError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// get performs GET and decodes JSON into out. Returns apiError on 4xx/5xx.
func (c *Client) get(ctx context.Context, baseURL, path string, out interface{}) error {
	url := baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		msg := string(body)
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &errBody)
		if errBody.Error != "" {
			msg = errBody.Error
		}
		return &apiError{StatusCode: resp.StatusCode, Body: string(body), Message: msg}
	}
	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// post performs POST and decodes JSON into out. Returns apiError on 4xx/5xx.
func (c *Client) post(ctx context.Context, baseURL, path string, out interface{}) error {
	return c.postBody(ctx, baseURL, path, nil, out)
}

// postBody performs POST with JSON body and decodes response into out.
func (c *Client) postBody(ctx context.Context, baseURL, path string, body interface{}, out interface{}) error {
	url := baseURL + path
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bodyReader)
	if err != nil {
		return err
	}
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		msg := string(respBody)
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(respBody, &errBody)
		if errBody.Error != "" {
			msg = errBody.Error
		}
		return &apiError{StatusCode: resp.StatusCode, Body: string(respBody), Message: msg}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// put performs PUT with JSON body and decodes response into out.
func (c *Client) put(ctx context.Context, baseURL, path string, body interface{}, out interface{}) error {
	url := baseURL + path
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bodyReader)
	if err != nil {
		return err
	}
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		msg := string(respBody)
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(respBody, &errBody)
		if errBody.Error != "" {
			msg = errBody.Error
		}
		return &apiError{StatusCode: resp.StatusCode, Body: string(respBody), Message: msg}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// delete performs DELETE. Returns apiError on 4xx/5xx.
func (c *Client) delete(ctx context.Context, baseURL, path string) error {
	url := baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		msg := string(body)
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &errBody)
		if errBody.Error != "" {
			msg = errBody.Error
		}
		return &apiError{StatusCode: resp.StatusCode, Body: msg, Message: msg}
	}
	return nil
}

// IsAPIError returns true if err is from an API 4xx/5xx response.
func IsAPIError(err error) (statusCode int, ok bool) {
	if e, ok := err.(*apiError); ok {
		return e.StatusCode, true
	}
	return 0, false
}

// ErrMessage returns a short message for the user (API error or err.Error()).
func ErrMessage(err error) string {
	if e, ok := err.(*apiError); ok {
		return e.Message
	}
	return err.Error()
}

// HealthResult is the result of GET /health for one service.
type HealthResult struct {
	OK    bool
	Error string
}

// DatasetHealth calls GET /health on dataset-service.
func (c *Client) DatasetHealth(ctx context.Context) HealthResult {
	return c.health(ctx, c.dataset)
}

// ScopeHealth calls GET /health on scope-service.
func (c *Client) ScopeHealth(ctx context.Context) HealthResult {
	return c.health(ctx, c.scope)
}

// RoutingHealth calls GET /health on routing-service.
func (c *Client) RoutingHealth(ctx context.Context) HealthResult {
	return c.health(ctx, c.routing)
}

// TargetHealth calls GET /health on target-service.
func (c *Client) TargetHealth(ctx context.Context) HealthResult {
	return c.health(ctx, c.target)
}

// CampaignHealth calls GET /health on campaign-service.
func (c *Client) CampaignHealth(ctx context.Context) HealthResult {
	return c.health(ctx, c.campaign)
}

func (c *Client) health(ctx context.Context, baseURL string) HealthResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return HealthResult{OK: false, Error: err.Error()}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return HealthResult{OK: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return HealthResult{OK: false, Error: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return HealthResult{OK: true}
}
