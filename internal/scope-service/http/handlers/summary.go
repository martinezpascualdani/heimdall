package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/pkg/iso3166"
)

// SummaryStore is the storage interface required by CountrySummaryHandler.
type SummaryStore interface {
	GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error)
	GetRegistriesByDatasetIDs(datasetIDs []uuid.UUID) (map[uuid.UUID]string, error)
	HasImportedDataset(datasetID uuid.UUID) (bool, error)
	CountBlocksByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, addressFamily string) (int64, error)
}

// CountrySummaryHandler handles GET /v1/scopes/country/{cc}/summary.
type CountrySummaryHandler struct {
	Store          SummaryStore
	DatasetBaseURL string
	DatasetClient  *http.Client
}

// summaryResponse is the response for the summary endpoint.
type summaryResponse struct {
	ScopeType       string             `json:"scope_type"`
	ScopeValue      string             `json:"scope_value"`
	DatasetsUsed    []datasetUsedItem  `json:"datasets_used"`
	DatasetID       *uuid.UUID         `json:"dataset_id,omitempty"` // present only when a single snapshot is used (backward compat)
	Registry        string             `json:"registry,omitempty"`
	Serial          string             `json:"serial,omitempty"`
	IPv4BlockCount  int64              `json:"ipv4_block_count"`
	IPv6BlockCount  int64              `json:"ipv6_block_count"`
	Total           int64              `json:"total"`
}

// ServeHTTP implements http.Handler.
func (h *CountrySummaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	ipv4, err := h.Store.CountBlocksByScope("country", cc, datasetIDs, "ipv4")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ipv6, err := h.Store.CountBlocksByScope("country", cc, datasetIDs, "ipv6")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	registries, _ := h.Store.GetRegistriesByDatasetIDs(datasetIDs)
	datasetsUsed := buildDatasetsUsed(h.DatasetClient, h.DatasetBaseURL, datasetIDs, registries)

	out := summaryResponse{
		ScopeType:      "country",
		ScopeValue:     cc,
		DatasetsUsed:   datasetsUsed,
		IPv4BlockCount: ipv4,
		IPv6BlockCount: ipv6,
		Total:          ipv4 + ipv6,
	}
	if len(datasetIDs) == 1 {
		out.DatasetID = &datasetIDs[0]
		if len(datasetsUsed) > 0 {
			out.Registry = datasetsUsed[0].Registry
			out.Serial = datasetsUsed[0].Serial
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(out)
}
