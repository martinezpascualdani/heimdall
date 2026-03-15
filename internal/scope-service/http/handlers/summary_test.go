package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type mockSummaryStore struct {
	latestIDs   []uuid.UUID
	hasImported bool
	ipv4Count   int64
	ipv6Count   int64
	hasErr      error
	latestErr   error
	countErr    error
}

func (m *mockSummaryStore) GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error) {
	if m.latestErr != nil {
		return nil, m.latestErr
	}
	return m.latestIDs, nil
}
func (m *mockSummaryStore) GetRegistriesByDatasetIDs(datasetIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string)
	for _, id := range datasetIDs {
		out[id] = "test"
	}
	return out, nil
}
func (m *mockSummaryStore) HasImportedDataset(id uuid.UUID) (bool, error) {
	if m.hasErr != nil {
		return false, m.hasErr
	}
	return m.hasImported, nil
}
func (m *mockSummaryStore) CountBlocksByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, addressFamily string) (int64, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	if addressFamily == "ipv4" {
		return m.ipv4Count, nil
	}
	return m.ipv6Count, nil
}

func TestCountrySummaryHandler_200(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountrySummaryHandler{Store: &mockSummaryStore{latestIDs: []uuid.UUID{id}, hasImported: true, ipv4Count: 5, ipv6Count: 3}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/summary", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body summaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ScopeType != "country" || body.ScopeValue != "ES" || body.DatasetID == nil || *body.DatasetID != id {
		t.Errorf("unexpected scope/dataset: %+v", body)
	}
	if len(body.DatasetsUsed) != 1 || body.DatasetsUsed[0].DatasetID != id {
		t.Errorf("expected datasets_used with one element; got %d", len(body.DatasetsUsed))
	}
	if body.IPv4BlockCount != 5 || body.IPv6BlockCount != 3 || body.Total != 8 {
		t.Errorf("expected ipv4=5 ipv6=3 total=8; got ipv4=%d ipv6=%d total=%d", body.IPv4BlockCount, body.IPv6BlockCount, body.Total)
	}
}

func TestCountrySummaryHandler_NoDataset(t *testing.T) {
	h := &CountrySummaryHandler{Store: &mockSummaryStore{latestIDs: nil}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/summary", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "no_dataset_available" {
		t.Errorf("expected no_dataset_available, got %s", body["error"])
	}
}

func TestCountrySummaryHandler_DatasetNotImported(t *testing.T) {
	datasetID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	h := &CountrySummaryHandler{Store: &mockSummaryStore{hasImported: false}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/summary", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/summary?dataset_id="+datasetID.String(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "dataset_not_imported" {
		t.Errorf("expected dataset_not_imported, got %s", body["error"])
	}
}

func TestCountrySummaryHandler_ZeroBlocks(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountrySummaryHandler{Store: &mockSummaryStore{latestIDs: []uuid.UUID{id}, hasImported: true, ipv4Count: 0, ipv6Count: 0}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/summary", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid country+dataset but 0 blocks, got %d", rec.Code)
	}
	var body summaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.IPv4BlockCount != 0 || body.IPv6BlockCount != 0 || body.Total != 0 {
		t.Errorf("expected counts 0; got ipv4=%d ipv6=%d total=%d", body.IPv4BlockCount, body.IPv6BlockCount, body.Total)
	}
}

func TestCountrySummaryHandler_InvalidCountry(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountrySummaryHandler{Store: &mockSummaryStore{latestIDs: []uuid.UUID{id}, hasImported: true}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/summary", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ZZ/summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "invalid_country_code" {
		t.Errorf("expected invalid_country_code, got %s", body["error"])
	}
}
