package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/ipresolver"
)

type mockResolver struct {
	result *ipresolver.ResolveResult
	err    error
}

func (m *mockResolver) Resolve(ctx context.Context, ipStr string, datasetID *uuid.UUID) (*ipresolver.ResolveResult, error) {
	return m.result, m.err
}

func TestIPResolveHandler_InvalidIP(t *testing.T) {
	h := &IPResolveHandler{Resolver: &mockResolver{err: ipresolver.ErrInvalidIP}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/by-ip/{ip}", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/by-ip/hola", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "invalid_ip" {
		t.Errorf("expected error invalid_ip, got %s", body["error"])
	}
}

func TestIPResolveHandler_NotFound(t *testing.T) {
	h := &IPResolveHandler{Resolver: &mockResolver{result: nil, err: nil}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/by-ip/{ip}", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/by-ip/8.8.8.8", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "ip_not_found" {
		t.Errorf("expected error ip_not_found, got %s", body["error"])
	}
}

func TestIPResolveHandler_NoDataset(t *testing.T) {
	h := &IPResolveHandler{Resolver: &mockResolver{result: nil, err: ipresolver.ErrNoDataset}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/by-ip/{ip}", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/by-ip/8.8.8.8", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "no_dataset_available" {
		t.Errorf("expected error no_dataset_available, got %s", body["error"])
	}
}

func TestIPResolveHandler_OK(t *testing.T) {
	id := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	h := &IPResolveHandler{
		Resolver: &mockResolver{
			result: &ipresolver.ResolveResult{IP: "8.8.8.8", ScopeType: "country", ScopeValue: "US", DatasetID: id},
			err:    nil,
		},
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/by-ip/{ip}", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/by-ip/8.8.8.8", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body Response
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.IP != "8.8.8.8" || body.ScopeType != "country" || body.ScopeValue != "US" || body.DatasetID != id {
		t.Errorf("unexpected body: %+v", body)
	}
}
