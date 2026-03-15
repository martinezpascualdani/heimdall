package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/pkg/iso3166"
)

// ASNSummaryStore is the storage interface required by CountryASNSummaryHandler.
type ASNSummaryStore interface {
	GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error)
	GetRegistriesByDatasetIDs(datasetIDs []uuid.UUID) (map[uuid.UUID]string, error)
	HasImportedDataset(datasetID uuid.UUID) (bool, error)
	CountASNRangeByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID) (int64, error)
	SumASNCountByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID) (int64, error)
}

// CountryASNSummaryHandler handles GET /v1/scopes/country/{cc}/asn-summary.
// Response includes datasets_used (required). asn_range_count = number of ASN range rows; asn_total_count = sum of ASNs across ranges.
type CountryASNSummaryHandler struct {
	Store          ASNSummaryStore
	DatasetBaseURL string
	DatasetClient  *http.Client
}

// asnSummaryResponse is the response for the ASN summary endpoint.
type asnSummaryResponse struct {
	ScopeType       string             `json:"scope_type"`
	ScopeValue      string             `json:"scope_value"`
	DatasetsUsed    []datasetUsedItem  `json:"datasets_used"` // required
	ASNRangeCount   int64              `json:"asn_range_count"`
	ASNTotalCount   int64              `json:"asn_total_count"`
}

// ServeHTTP implements http.Handler.
func (h *CountryASNSummaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	rangeCount, err := h.Store.CountASNRangeByScope("country", cc, datasetIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, err := h.Store.SumASNCountByScope("country", cc, datasetIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	registries, _ := h.Store.GetRegistriesByDatasetIDs(datasetIDs)
	datasetsUsed := buildDatasetsUsed(h.DatasetClient, h.DatasetBaseURL, datasetIDs, registries)

	out := asnSummaryResponse{
		ScopeType:     "country",
		ScopeValue:    cc,
		DatasetsUsed:  datasetsUsed,
		ASNRangeCount: rangeCount,
		ASNTotalCount: totalCount,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(out)
}
