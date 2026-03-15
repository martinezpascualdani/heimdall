package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/martinezpascualdani/heimdall/internal/heimdallctl/config"
)

func TestClient_Get_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/datasets" {
			t.Errorf("path: got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"datasets":[{"id":"a","state":"validated","source":"ripencc"}]}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		DatasetURL: ts.URL,
		ScopeURL:   "http://localhost:8081",
		RoutingURL: "http://localhost:8082",
		Timeout:    5 * time.Second,
	}
	cl := New(cfg)
	ctx := context.Background()

	var out DatasetListResponse
	err := cl.get(ctx, cl.dataset, "/v1/datasets", &out)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(out.Datasets) != 1 {
		t.Fatalf("datasets: got %d", len(out.Datasets))
	}
	if out.Datasets[0].ID != "a" || out.Datasets[0].Source != "ripencc" {
		t.Errorf("dataset: got id=%q source=%q", out.Datasets[0].ID, out.Datasets[0].Source)
	}
}

func TestClient_Get_4xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"dataset not found"}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		DatasetURL: ts.URL,
		ScopeURL:   "http://localhost:8081",
		RoutingURL: "http://localhost:8082",
		Timeout:    5 * time.Second,
	}
	cl := New(cfg)
	ctx := context.Background()

	var out DatasetVersion
	err := cl.get(ctx, cl.dataset, "/v1/datasets/missing", &out)
	if err == nil {
		t.Fatal("expected error")
	}
	code, ok := IsAPIError(err)
	if !ok {
		t.Fatalf("expected apiError, got %T", err)
	}
	if code != http.StatusNotFound {
		t.Errorf("status: got %d", code)
	}
	if ErrMessage(err) != "dataset not found" {
		t.Errorf("message: got %q", ErrMessage(err))
	}
}

func TestClient_Post_4xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"dataset_id required"}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		DatasetURL: ts.URL,
		ScopeURL:   ts.URL,
		RoutingURL: ts.URL,
		Timeout:    5 * time.Second,
	}
	cl := New(cfg)
	ctx := context.Background()

	_, err := cl.ScopeSync(ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	code, ok := IsAPIError(err)
	if !ok {
		t.Fatalf("expected apiError, got %T", err)
	}
	if code != http.StatusBadRequest {
		t.Errorf("status: got %d", code)
	}
}

func TestClient_Health(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		DatasetURL: ts.URL,
		ScopeURL:   ts.URL,
		RoutingURL: ts.URL,
		Timeout:    2 * time.Second,
	}
	cl := New(cfg)
	ctx := context.Background()

	r := cl.DatasetHealth(ctx)
	if !r.OK {
		t.Errorf("DatasetHealth: got OK=false, error=%q", r.Error)
	}
}

func TestClient_Health_Unreachable(t *testing.T) {
	cfg := &config.Config{
		DatasetURL: "http://127.0.0.1:31999",
		ScopeURL:   "http://127.0.0.1:31999",
		RoutingURL: "http://127.0.0.1:31999",
		Timeout:    100 * time.Millisecond,
	}
	cl := New(cfg)
	ctx := context.Background()

	r := cl.DatasetHealth(ctx)
	if r.OK {
		t.Error("expected OK=false for unreachable server")
	}
	if r.Error == "" {
		t.Error("expected non-empty error")
	}
}
