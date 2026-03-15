package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// No env, no file: must get defaults. Use temp dir as HOME so primary path doesn't exist.
	os.Clearenv()
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	if err := os.Chdir(dir); err != nil {
		t.Skipf("cannot chdir: %v", err)
	}
	defer os.Chdir("/")

	c := Load()
	if c.DatasetURL != DefaultDatasetURL {
		t.Errorf("DatasetURL: got %q, want %q", c.DatasetURL, DefaultDatasetURL)
	}
	if c.ScopeURL != DefaultScopeURL {
		t.Errorf("ScopeURL: got %q, want %q", c.ScopeURL, DefaultScopeURL)
	}
	if c.RoutingURL != DefaultRoutingURL {
		t.Errorf("RoutingURL: got %q, want %q", c.RoutingURL, DefaultRoutingURL)
	}
	if c.TargetURL != DefaultTargetURL {
		t.Errorf("TargetURL: got %q, want %q", c.TargetURL, DefaultTargetURL)
	}
	if c.Timeout != DefaultTimeout {
		t.Errorf("Timeout: got %v, want %v", c.Timeout, DefaultTimeout)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	os.Clearenv()
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	if err := os.Chdir(dir); err != nil {
		t.Skipf("cannot chdir: %v", err)
	}
	defer os.Chdir("/")
	os.Setenv("HEIMDALL_DATASET_URL", "http://dataset:8080")
	os.Setenv("HEIMDALL_SCOPE_URL", "http://scope:8081")
	os.Setenv("HEIMDALL_ROUTING_URL", "http://routing:8082")
	os.Setenv("HEIMDALL_TARGET_URL", "http://target:8083")
	os.Setenv("HEIMDALL_TIMEOUT", "60")

	c := Load()
	if c.DatasetURL != "http://dataset:8080" {
		t.Errorf("DatasetURL: got %q", c.DatasetURL)
	}
	if c.ScopeURL != "http://scope:8081" {
		t.Errorf("ScopeURL: got %q", c.ScopeURL)
	}
	if c.RoutingURL != "http://routing:8082" {
		t.Errorf("RoutingURL: got %q", c.RoutingURL)
	}
	if c.TargetURL != "http://target:8083" {
		t.Errorf("TargetURL: got %q", c.TargetURL)
	}
	if c.Timeout != 60*time.Second {
		t.Errorf("Timeout: got %v", c.Timeout)
	}
}

func TestLoad_FallbackFile(t *testing.T) {
	os.Clearenv()
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	if err := os.Chdir(dir); err != nil {
		t.Skipf("cannot chdir: %v", err)
	}
	defer os.Chdir("/")
	fallback := filepath.Join(dir, ".heimdall.yaml")
	content := `dataset_url: http://from-file-dataset:8080
scope_url: http://from-file-scope:8081
routing_url: http://from-file-routing:8082
target_url: http://from-file-target:8083
timeout_seconds: 45
`
	if err := os.WriteFile(fallback, []byte(content), 0644); err != nil {
		t.Fatalf("write fallback: %v", err)
	}

	c := Load()
	if c.DatasetURL != "http://from-file-dataset:8080" {
		t.Errorf("DatasetURL: got %q", c.DatasetURL)
	}
	if c.ScopeURL != "http://from-file-scope:8081" {
		t.Errorf("ScopeURL: got %q", c.ScopeURL)
	}
	if c.RoutingURL != "http://from-file-routing:8082" {
		t.Errorf("RoutingURL: got %q", c.RoutingURL)
	}
	if c.TargetURL != "http://from-file-target:8083" {
		t.Errorf("TargetURL: got %q", c.TargetURL)
	}
	if c.Timeout != 45*time.Second {
		t.Errorf("Timeout: got %v", c.Timeout)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	os.Clearenv()
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	if err := os.Chdir(dir); err != nil {
		t.Skipf("cannot chdir: %v", err)
	}
	defer os.Chdir("/")
	os.Setenv("HEIMDALL_DATASET_URL", "http://env-dataset:8080")
	fallback := filepath.Join(dir, ".heimdall.yaml")
	if err := os.WriteFile(fallback, []byte("dataset_url: http://file-dataset:8080\n"), 0644); err != nil {
		t.Fatalf("write fallback: %v", err)
	}

	c := Load()
	// Env must win
	if c.DatasetURL != "http://env-dataset:8080" {
		t.Errorf("DatasetURL: env should override file, got %q", c.DatasetURL)
	}
}
