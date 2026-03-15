package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type mockASNSummaryStore struct {
	latestIDs     []uuid.UUID
	hasImported   bool
	rangeCount    int64
	totalCount    int64
	countErr      error
	sumErr        error
	hasErr        error
	latestErr     error
}

func (m *mockASNSummaryStore) GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error) {
	if m.latestErr != nil {
		return nil, m.latestErr
	}
	return m.latestIDs, nil
}
func (m *mockASNSummaryStore) GetRegistriesByDatasetIDs(datasetIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string)
	for _, id := range datasetIDs {
		out[id] = "ripencc"
	}
	return out, nil
}
func (m *mockASNSummaryStore) HasImportedDataset(id uuid.UUID) (bool, error) {
	if m.hasErr != nil {
		return false, m.hasErr
	}
	return m.hasImported, nil
}
func (m *mockASNSummaryStore) CountASNRangeByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID) (int64, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return m.rangeCount, nil
}
func (m *mockASNSummaryStore) SumASNCountByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID) (int64, error) {
	if m.sumErr != nil {
		return 0, m.sumErr
	}
	return m.totalCount, nil
}

func TestCountryASNSummaryHandler_InvalidCountry(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryASNSummaryHandler{Store: &mockASNSummaryStore{latestIDs: []uuid.UUID{id}}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/asn-summary", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ZZ/asn-summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid country: expected 400, got %d", rec.Code)
	}
}

func TestCountryASNSummaryHandler_ValidCountryNoASNs_Returns200WithZeros(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryASNSummaryHandler{
		Store: &mockASNSummaryStore{latestIDs: []uuid.UUID{id}, rangeCount: 0, totalCount: 0},
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/asn-summary", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/asn-summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("valid country with no ASNs: expected 200, got %d", rec.Code)
	}
	var body asnSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ASNRangeCount != 0 || body.ASNTotalCount != 0 {
		t.Errorf("expected asn_range_count=0 asn_total_count=0, got %d %d", body.ASNRangeCount, body.ASNTotalCount)
	}
	if len(body.DatasetsUsed) == 0 {
		t.Error("datasets_used should be present (required)")
	}
}

func TestCountryASNSummaryHandler_CountsCorrect(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	// e.g. 3 ranges with asn_count 1, 10, 100 → range_count=3, total_count=111
	h := &CountryASNSummaryHandler{
		Store: &mockASNSummaryStore{
			latestIDs:  []uuid.UUID{id},
			rangeCount: 3,
			totalCount: 111,
		},
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/asn-summary", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/asn-summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body asnSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ASNRangeCount != 3 || body.ASNTotalCount != 111 {
		t.Errorf("expected asn_range_count=3 asn_total_count=111, got %d %d", body.ASNRangeCount, body.ASNTotalCount)
	}
}

func TestCountryASNSummaryHandler_DatasetIDNotImported_Returns404(t *testing.T) {
	h := &CountryASNSummaryHandler{Store: &mockASNSummaryStore{hasImported: false}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/asn-summary", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/asn-summary?dataset_id=11111111-1111-1111-1111-111111111111", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("dataset not imported: expected 404, got %d", rec.Code)
	}
}
