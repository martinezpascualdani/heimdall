package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/domain"
	"github.com/martinezpascualdani/heimdall/pkg/iso3166"
)

const (
	asnDefaultLimit = 1000
	asnMaxLimit     = 100_000
)

// ASNsStore is the storage interface required by CountryASNsHandler (no SumASNCountByScope).
type ASNsStore interface {
	GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error)
	GetRegistriesByDatasetIDs(datasetIDs []uuid.UUID) (map[uuid.UUID]string, error)
	HasImportedDataset(datasetID uuid.UUID) (bool, error)
	ListASNsByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, limit, offset int) ([]*domain.ScopeASN, error)
	CountASNRangeByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID) (int64, error)
}

// CountryASNsHandler handles GET /v1/scopes/country/{cc}/asns.
// Without dataset_id uses latest imported version per registry (multi-RIR); total = number of ASN ranges.
type CountryASNsHandler struct {
	Store          ASNsStore
	DatasetBaseURL string
	DatasetClient  *http.Client
}

// asnsResponse is the response for the ASNs endpoint.
type asnsResponse struct {
	ScopeType    string             `json:"scope_type"`
	ScopeValue   string             `json:"scope_value"`
	DatasetsUsed []datasetUsedItem  `json:"datasets_used"`
	Count        int                `json:"count"`
	Total        int64              `json:"total"` // total ASN ranges (not individual ASNs)
	Limit        int                `json:"limit"`
	Offset       int                `json:"offset"`
	HasMore      bool               `json:"has_more"`
	Items        []asnItem          `json:"items"`
}

// asnItem is one ASN range in the API response. asn_end = asn_start + asn_count - 1 (computed).
type asnItem struct {
	ASNStart  int64  `json:"asn_start"`
	ASNCount  int64  `json:"asn_count"`
	ASNEnd    int64  `json:"asn_end"`
	Status    string `json:"status"`
	CC        string `json:"cc"`
	Date      string `json:"date"` // raw YYYYMMDD
}

// ServeHTTP implements http.Handler.
func (h *CountryASNsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ccRaw := r.PathValue("cc")
	if ccRaw == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_country_code")
		return
	}
	cc := strings.ToUpper(ccRaw)
	if !iso3166.ValidAlpha2(cc) {
		writeJSONError(w, http.StatusBadRequest, "invalid_country_code")
		return
	}

	var datasetIDs []uuid.UUID
	if idStr := r.URL.Query().Get("dataset_id"); idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_dataset_id")
			return
		}
		ok, err := h.Store.HasImportedDataset(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			writeJSONError(w, http.StatusNotFound, "dataset_not_imported")
			return
		}
		datasetIDs = []uuid.UUID{id}
	} else {
		ids, err := h.Store.GetLatestImportedDatasetIDsPerRegistry()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(ids) == 0 {
			writeJSONError(w, http.StatusServiceUnavailable, "no_dataset_available")
			return
		}
		datasetIDs = ids
	}

	limit := asnDefaultLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 0 || n > asnMaxLimit {
			writeJSONError(w, http.StatusBadRequest, "invalid_limit_or_offset")
			return
		}
		limit = n
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		n, err := strconv.Atoi(o)
		if err != nil || n < 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid_limit_or_offset")
			return
		}
		offset = n
	}

	total, err := h.Store.CountASNRangeByScope("country", cc, datasetIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []*domain.ScopeASN
	if offset < int(total) {
		items, err = h.Store.ListASNsByScope("country", cc, datasetIDs, limit, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	registries, _ := h.Store.GetRegistriesByDatasetIDs(datasetIDs)
	datasetsUsed := buildDatasetsUsed(h.DatasetClient, h.DatasetBaseURL, datasetIDs, registries)

	asnItems := make([]asnItem, len(items))
	for i, a := range items {
		asnEnd := a.ASNStart + a.ASNCount - 1
		asnItems[i] = asnItem{
			ASNStart: a.ASNStart,
			ASNCount: a.ASNCount,
			ASNEnd:   asnEnd,
			Status:   a.Status,
			CC:       a.CC,
			Date:     a.Date,
		}
	}

	out := asnsResponse{
		ScopeType:    "country",
		ScopeValue:   cc,
		DatasetsUsed: datasetsUsed,
		Count:        len(asnItems),
		Total:        total,
		Limit:        limit,
		Offset:       offset,
		HasMore:      offset+len(asnItems) < int(total),
		Items:        asnItems,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(out)
}
