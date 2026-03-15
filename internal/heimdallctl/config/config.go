package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultDatasetURL  = "http://localhost:8080"
	DefaultScopeURL    = "http://localhost:8081"
	DefaultRoutingURL  = "http://localhost:8082"
	DefaultTargetURL   = "http://localhost:8083"
	DefaultCampaignURL = "http://localhost:8084"
	DefaultTimeout     = 30 * time.Second
)

// Config holds Heimdall service base URLs and HTTP timeout for heimdallctl.
type Config struct {
	DatasetURL  string        `yaml:"dataset_url" json:"dataset_url"`
	ScopeURL    string        `yaml:"scope_url" json:"scope_url"`
	RoutingURL  string        `yaml:"routing_url" json:"routing_url"`
	TargetURL   string        `yaml:"target_url" json:"target_url"`
	CampaignURL string        `yaml:"campaign_url" json:"campaign_url"`
	Timeout     time.Duration `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
}

// fileConfig is the YAML file shape (timeout in seconds).
type fileConfig struct {
	DatasetURL  string `yaml:"dataset_url"`
	ScopeURL    string `yaml:"scope_url"`
	RoutingURL  string `yaml:"routing_url"`
	TargetURL   string `yaml:"target_url"`
	CampaignURL string `yaml:"campaign_url"`
	TimeoutSec  int    `yaml:"timeout_seconds"`
}

// Load builds Config from defaults, then optional config files, then environment.
// Order: 4) defaults, 3) fallback .heimdall.yaml in cwd if exists, 2) primary ~/.config/heimdall/config.yaml if exists, 1) env (highest priority).
func Load() *Config {
	c := &Config{
		DatasetURL:  DefaultDatasetURL,
		ScopeURL:    DefaultScopeURL,
		RoutingURL:  DefaultRoutingURL,
		TargetURL:   DefaultTargetURL,
		CampaignURL: DefaultCampaignURL,
		Timeout:     DefaultTimeout,
	}

	// Fallback: .heimdall.yaml in cwd (lower priority)
	if wd, err := os.Getwd(); err == nil {
		fallback := filepath.Join(wd, ".heimdall.yaml")
		applyFile(c, fallback)
	}

	// Primary: ~/.config/heimdall/config.yaml (overrides fallback)
	if dir, err := os.UserHomeDir(); err == nil {
		primary := filepath.Join(dir, ".config", "heimdall", "config.yaml")
		applyFile(c, primary)
	}

	// Env (highest priority)
	if v := os.Getenv("HEIMDALL_DATASET_URL"); v != "" {
		c.DatasetURL = v
	}
	if v := os.Getenv("HEIMDALL_SCOPE_URL"); v != "" {
		c.ScopeURL = v
	}
	if v := os.Getenv("HEIMDALL_ROUTING_URL"); v != "" {
		c.RoutingURL = v
	}
	if v := os.Getenv("HEIMDALL_TARGET_URL"); v != "" {
		c.TargetURL = v
	}
	if v := os.Getenv("HEIMDALL_CAMPAIGN_URL"); v != "" {
		c.CampaignURL = v
	}
	if v := os.Getenv("HEIMDALL_TIMEOUT"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			c.Timeout = time.Duration(sec) * time.Second
		}
	}

	return c
}

// applyFile reads the YAML file and applies non-empty values to c. Returns true if file was read successfully.
func applyFile(c *Config, path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var f fileConfig
	if err := yaml.Unmarshal(data, &f); err != nil {
		return true // file existed, consider it applied (skip fallback)
	}
	if f.DatasetURL != "" {
		c.DatasetURL = f.DatasetURL
	}
	if f.ScopeURL != "" {
		c.ScopeURL = f.ScopeURL
	}
	if f.RoutingURL != "" {
		c.RoutingURL = f.RoutingURL
	}
	if f.TargetURL != "" {
		c.TargetURL = f.TargetURL
	}
	if f.CampaignURL != "" {
		c.CampaignURL = f.CampaignURL
	}
	if f.TimeoutSec > 0 {
		c.Timeout = time.Duration(f.TimeoutSec) * time.Second
	}
	return true
}
