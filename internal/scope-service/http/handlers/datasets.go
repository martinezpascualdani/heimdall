package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/storage"
	"github.com/martinezpascualdani/heimdall/pkg/iso3166"
)

// DatasetsStore is the storage interface required by CountryDatasetsHandler.
type DatasetsStore interface {
	ListImportedDatasetsForScope(scopeType, scopeValue string) ([]storage.ImportSummary, error)
}

// CountryDatasetsHandler handles GET /v1/scopes/country/{cc}/datasets.
type CountryDatasetsHandler struct {
	Store          DatasetsStore
	DatasetBaseURL string
	DatasetClient  *http.Client
}

// datasetItem is one element in the /datasets response (dataset_id and created_at required; registry, serial optional).
type datasetItem struct {
	DatasetID  uuid.UUID `json:"dataset_id"`
	CreatedAt  time.Time `json:"created_at"`
	Registry   string    `json:"registry,omitempty"`
	Serial     string    `json:"serial,omitempty"`
}

// ServeHTTP implements http.Handler.
func (h *CountryDatasetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	list, err := h.Store.ListImportedDatasetsForScope("country", cc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]datasetItem, len(list))
	for i, sum := range list {
		items[i] = datasetItem{DatasetID: sum.DatasetID, CreatedAt: sum.CreatedAt}
		if h.DatasetBaseURL != "" && h.DatasetClient != nil {
			if meta := fetchDatasetMetaWithTimeout(h.DatasetClient, h.DatasetBaseURL, sum.DatasetID); meta != nil {
				items[i].Registry = meta.Registry
				items[i].Serial = strconv.FormatInt(meta.Serial, 10)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
}
