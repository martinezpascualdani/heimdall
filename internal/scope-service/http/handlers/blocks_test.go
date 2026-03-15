package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/domain"
)

type mockBlocksStore struct {
	latestIDs   []uuid.UUID // returned by GetLatestImportedDatasetIDsPerRegistry when no dataset_id param
	hasImported bool
	blocks      []*domain.ScopeBlock
	total       int64
	listErr     error
	countErr    error
	hasErr      error
	latestErr   error
}

func (m *mockBlocksStore) GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error) {
	if m.latestErr != nil {
		return nil, m.latestErr
	}
	return m.latestIDs, nil
}
func (m *mockBlocksStore) GetRegistriesByDatasetIDs(datasetIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string)
	for _, id := range datasetIDs {
		out[id] = "test"
	}
	return out, nil
}
func (m *mockBlocksStore) HasImportedDataset(id uuid.UUID) (bool, error) {
	if m.hasErr != nil {
		return false, m.hasErr
	}
	return m.hasImported, nil
}
func (m *mockBlocksStore) ListBlocksByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, addressFamily string, limit, offset int) ([]*domain.ScopeBlock, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.blocks, nil
}
func (m *mockBlocksStore) CountBlocksByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, addressFamily string) (int64, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return m.total, nil
}

func TestCountryBlocksHandler_InvalidCountry(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryBlocksHandler{Store: &mockBlocksStore{latestIDs: []uuid.UUID{id}, hasImported: true, total: 0}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	for _, cc := range []string{"ZZ", "QQ", "e1", "123", "A"} {
		req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/"+cc+"/blocks", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("cc=%q: expected 400, got %d", cc, rec.Code)
		}
		var body map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["error"] != "invalid_country_code" {
			t.Errorf("cc=%q: expected invalid_country_code, got %s", cc, body["error"])
		}
	}
}

func TestCountryBlocksHandler_ValidCountryNormalized(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryBlocksHandler{Store: &mockBlocksStore{latestIDs: []uuid.UUID{id}, hasImported: true, total: 0, blocks: []*domain.ScopeBlock{}}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/es/blocks", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for es (normalized to ES), got %d", rec.Code)
	}
	var body blocksResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ScopeValue != "ES" {
		t.Errorf("expected scope_value ES, got %s", body.ScopeValue)
	}
}

func TestCountryBlocksHandler_NoDataset(t *testing.T) {
	h := &CountryBlocksHandler{Store: &mockBlocksStore{latestIDs: nil}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/blocks", nil)
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
		t.Errorf("expected no_dataset_available, got %s", body["error"])
	}
}

func TestCountryBlocksHandler_DatasetNotImported(t *testing.T) {
	datasetID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	h := &CountryBlocksHandler{Store: &mockBlocksStore{hasImported: false}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/blocks?dataset_id="+datasetID.String(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "dataset_not_imported" {
		t.Errorf("expected dataset_not_imported, got %s", body["error"])
	}
}

func TestCountryBlocksHandler_InvalidDatasetID(t *testing.T) {
	h := &CountryBlocksHandler{Store: &mockBlocksStore{}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/blocks?dataset_id=not-a-uuid", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	var m map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m["error"] != "invalid_dataset_id" {
		t.Errorf("expected invalid_dataset_id, got %v", m)
	}
}

func TestCountryBlocksHandler_InvalidAddressFamily(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryBlocksHandler{Store: &mockBlocksStore{latestIDs: []uuid.UUID{id}, hasImported: true}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/blocks?address_family=ipv5", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "invalid_address_family" {
		t.Errorf("expected invalid_address_family, got %s", body["error"])
	}
}

func TestCountryBlocksHandler_AddressFamilyCaseInsensitive(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryBlocksHandler{Store: &mockBlocksStore{latestIDs: []uuid.UUID{id}, hasImported: true, total: 1, blocks: []*domain.ScopeBlock{
		{AddressFamily: "ipv4", Start: "1.0.0.0", Value: "256", Status: "allocated", NormalizedCIDRs: []string{}},
	}}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/blocks?address_family=IPv4", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for address_family=IPv4, got %d", rec.Code)
	}
	var body blocksResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.AddressFamily != "ipv4" {
		t.Errorf("expected address_family ipv4 in response, got %s", body.AddressFamily)
	}
	if len(body.Items) != 1 || body.Items[0].AddressFamily != "ipv4" {
		t.Errorf("expected one ipv4 item, got %d items", len(body.Items))
	}
}

func TestCountryBlocksHandler_LimitZero(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryBlocksHandler{Store: &mockBlocksStore{latestIDs: []uuid.UUID{id}, hasImported: true, total: 100, blocks: []*domain.ScopeBlock{}}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/blocks?limit=0", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for limit=0, got %d", rec.Code)
	}
	var body blocksResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 0 || len(body.Items) != 0 || body.Total != 100 || body.Limit != 0 {
		t.Errorf("expected count=0, items=[], total=100, limit=0; got count=%d total=%d limit=%d items=%d", body.Count, body.Total, body.Limit, len(body.Items))
	}
}

func TestCountryBlocksHandler_OffsetBeyondTotal(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryBlocksHandler{Store: &mockBlocksStore{latestIDs: []uuid.UUID{id}, hasImported: true, total: 10, blocks: []*domain.ScopeBlock{}}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/blocks?offset=100", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 when offset >= total, got %d", rec.Code)
	}
	var body blocksResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 0 || len(body.Items) != 0 || body.Total != 10 {
		t.Errorf("expected count=0, items=[], total=10; got count=%d total=%d items=%d", body.Count, body.Total, len(body.Items))
	}
}

func TestCountryBlocksHandler_ZeroBlocks(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryBlocksHandler{Store: &mockBlocksStore{latestIDs: []uuid.UUID{id}, hasImported: true, total: 0, blocks: []*domain.ScopeBlock{}}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/blocks", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for valid country+dataset but 0 blocks, got %d", rec.Code)
	}
	var body blocksResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 0 || body.Count != 0 || body.ScopeValue != "ES" || body.DatasetID == nil || *body.DatasetID != id {
		t.Errorf("expected total=0 count=0 scope_value=ES; got total=%d count=%d scope_value=%s", body.Total, body.Count, body.ScopeValue)
	}
}

func TestCountryBlocksHandler_200Structure(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	h := &CountryBlocksHandler{Store: &mockBlocksStore{
		latestIDs:   []uuid.UUID{id},
		hasImported: true,
		total:       1,
		blocks: []*domain.ScopeBlock{
			{AddressFamily: "ipv4", Start: "1.0.0.0", Value: "256", Status: "allocated", NormalizedCIDRs: []string{"1.0.0.0/24"}},
		},
	}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/scopes/country/{cc}/blocks", h)

	req := httptest.NewRequest(http.MethodGet, "http://test/v1/scopes/country/ES/blocks?limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var body blocksResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ScopeType != "country" || body.ScopeValue != "ES" || body.DatasetID == nil || *body.DatasetID != id {
		t.Errorf("unexpected scope/dataset: %+v", body)
	}
	if len(body.DatasetsUsed) != 1 || body.DatasetsUsed[0].DatasetID != id {
		t.Errorf("expected datasets_used with one element; got %d", len(body.DatasetsUsed))
	}
	if body.AddressFamily != "all" {
		t.Errorf("expected address_family all when no filter, got %s", body.AddressFamily)
	}
	if body.Count != 1 || body.Total != 1 || body.Limit != 10 || body.Offset != 0 {
		t.Errorf("unexpected count/total/limit/offset: count=%d total=%d limit=%d offset=%d", body.Count, body.Total, body.Limit, body.Offset)
	}
	if len(body.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(body.Items))
	}
	item := body.Items[0]
	if item.RawStart != "1.0.0.0" || item.RawValue != "256" || item.Status != "allocated" {
		t.Errorf("unexpected item: raw_start=%s raw_value=%s status=%s", item.RawStart, item.RawValue, item.Status)
	}
	if len(item.Normalized) != 1 || item.Normalized[0] != "1.0.0.0/24" {
		t.Errorf("expected normalized [1.0.0.0/24], got %v", item.Normalized)
	}
}
