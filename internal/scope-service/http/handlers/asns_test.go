package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/domain"
)

type mockASNsStore struct {
	latestIDs   []uuid.UUID
	hasImported bool
	asns        []*domain.ScopeASN
	total       int64
	listErr     error
	countErr    error
	hasErr      error
	latestErr   error
}

func (m *mockASNsStore) GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error) {
	if m.latestErr != nil {
		return nil, m.latestErr
	}
	return m.latestIDs, nil
}
func (m *mockASNsStore) GetRegistriesByDatasetIDs(datasetIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string)
	for _, id := range datasetIDs {
		out[id] = "ripencc"
	}
	return out, nil
}
func (m *mockASNsStore) HasImportedDataset(id uuid.UUID) (bool, error) {
	if m.hasErr != nil {
		return false, m.hasErr
	}
	return m.hasImported, nil
}
func (m *mockASNsStore) ListASNsByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, limit, offset int) ([]*domain.ScopeASN, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.asns, nil
}
func (m *mockASNsStore) CountASNRangeByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID) (int64, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return m.total, nil
}

func TestCountryASNsHandler_InvalidCountry(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryASNsHandler{Store: &mockASNsStore{latestIDs: []uuid.UUID{id}, total: 0}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/asns", h)

	for _, cc := range []string{"ZZ", "e1", "123"} {
		req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/"+cc+"/asns", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("cc=%q: expected 400, got %d", cc, rec.Code)
		}
	}
}

func TestCountryASNsHandler_ValidCountryNoASNs_Returns200(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryASNsHandler{Store: &mockASNsStore{latestIDs: []uuid.UUID{id}, asns: nil, total: 0}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/asns", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/asns", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("valid country with no ASNs: expected 200, got %d", rec.Code)
	}
	var body asnsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 0 || body.Total != 0 {
		t.Errorf("expected items=[], total=0; got count=%d total=%d", len(body.Items), body.Total)
	}
}

func TestCountryASNsHandler_DatasetIDNotImported_Returns404(t *testing.T) {
	h := &CountryASNsHandler{Store: &mockASNsStore{hasImported: false}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/asns", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/asns?dataset_id=11111111-1111-1111-1111-111111111111", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("dataset not imported: expected 404, got %d", rec.Code)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "dataset_not_imported" {
		t.Errorf("expected error dataset_not_imported, got %s", body["error"])
	}
}

func TestCountryASNsHandler_ASNCountOne_AsnEndEqualsAsnStart(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	singleASN := &domain.ScopeASN{
		DatasetID: id, ScopeType: "country", ScopeValue: "ES",
		ASNStart: 65536, ASNCount: 1, Status: "allocated", CC: "ES", Date: "20240101",
		RawIdentity: "ripencc|ES|asn|65536|1|20240101|allocated",
	}
	h := &CountryASNsHandler{
		Store: &mockASNsStore{
			latestIDs: []uuid.UUID{id},
			asns:     []*domain.ScopeASN{singleASN},
			total:    1,
		},
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/asns", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/asns", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body asnsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(body.Items))
	}
	if body.Items[0].ASNEnd != body.Items[0].ASNStart {
		t.Errorf("asn_count=1: expected asn_end == asn_start (%d), got asn_end=%d", body.Items[0].ASNStart, body.Items[0].ASNEnd)
	}
	if body.Items[0].ASNStart != 65536 || body.Items[0].ASNCount != 1 {
		t.Errorf("expected asn_start=65536 asn_count=1, got %d %d", body.Items[0].ASNStart, body.Items[0].ASNCount)
	}
}

func TestCountryASNsHandler_HasMore(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryASNsHandler{
		Store: &mockASNsStore{
			latestIDs: []uuid.UUID{id},
			asns:      []*domain.ScopeASN{}, // 0 items this page
			total:     5,
		},
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/asns", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/asns?limit=2&offset=2", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var body asnsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	// offset=2, count=0, total=5 → has_more = 2+0 < 5 = true
	if !body.HasMore {
		t.Errorf("expected has_more=true when offset+count < total (2+0<5), got false")
	}
}
