package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/domain"
	"github.com/martinezpascualdani/heimdall/pkg/iso3166"
)

const (
	defaultLimit = 1000
	maxLimit     = 100_000
	datasetMetaTimeout = 3 * time.Second
)

// BlocksStore is the storage interface required by CountryBlocksHandler.
type BlocksStore interface {
	GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error)
	GetRegistriesByDatasetIDs(datasetIDs []uuid.UUID) (map[uuid.UUID]string, error)
	HasImportedDataset(datasetID uuid.UUID) (bool, error)
	ListBlocksByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, addressFamily string, limit, offset int) ([]*domain.ScopeBlock, error)
	CountBlocksByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, addressFamily string) (int64, error)
}

// CountryBlocksHandler handles GET /v1/scopes/country/{cc}/blocks.
type CountryBlocksHandler struct {
	Store           BlocksStore
	DatasetBaseURL  string
	DatasetClient   *http.Client
}

// datasetUsedItem is one snapshot used in the response (when aggregating from multiple RIRs).
type datasetUsedItem struct {
	DatasetID uuid.UUID `json:"dataset_id"`
	Registry  string    `json:"registry,omitempty"`
	Serial    string    `json:"serial,omitempty"`
}

// blocksResponse is the unified response for the blocks endpoint.
type blocksResponse struct {
	ScopeType     string            `json:"scope_type"`
	ScopeValue    string            `json:"scope_value"`
	DatasetsUsed  []datasetUsedItem `json:"datasets_used"`
	DatasetID     *uuid.UUID        `json:"dataset_id,omitempty"` // present only when a single snapshot is used (backward compat)
	Registry      string            `json:"registry,omitempty"`
	Serial        string            `json:"serial,omitempty"`
	AddressFamily string            `json:"address_family"`
	Count         int               `json:"count"`
	Total         int64             `json:"total"`
	Limit         int               `json:"limit"`
	Offset        int               `json:"offset"`
	HasMore       bool              `json:"has_more"` // offset + count < total
	Items         []blockItem       `json:"items"`
}

// blockItem is a single block in the API response (raw_start, raw_value, normalized as strings).
type blockItem struct {
	AddressFamily string   `json:"address_family"`
	RawStart     string   `json:"raw_start"`
	RawValue     string   `json:"raw_value"`
	Status       string   `json:"status"`
	Normalized   []string `json:"normalized"`
}

// datasetMetaResponse is the subset we need from GET /v1/datasets/{id}.
type datasetMetaResponse struct {
	Registry string `json:"registry"`
	Serial   int64  `json:"serial"`
}

// ServeHTTP implements http.Handler.
func (h *CountryBlocksHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	addressFamily := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("address_family")))
	if addressFamily != "" && addressFamily != "ipv4" && addressFamily != "ipv6" {
		writeJSONError(w, http.StatusBadRequest, "invalid_address_family")
		return
	}

	limit := defaultLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 0 || n > maxLimit {
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

	total, err := h.Store.CountBlocksByScope("country", cc, datasetIDs, addressFamily)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If offset >= total, return 200 with empty items (plan: no error).
	var items []*domain.ScopeBlock
	if offset < int(total) {
		items, err = h.Store.ListBlocksByScope("country", cc, datasetIDs, addressFamily, limit, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	respAF := "all"
	if addressFamily != "" {
		respAF = addressFamily
	}

	blockItems := make([]blockItem, len(items))
	for i, b := range items {
		norm := b.NormalizedCIDRs
		if norm == nil {
			norm = []string{}
		}
		blockItems[i] = blockItem{
			AddressFamily: b.AddressFamily,
			RawStart:      b.Start,
			RawValue:      b.Value,
			Status:        b.Status,
			Normalized:    norm,
		}
	}

	registries, _ := h.Store.GetRegistriesByDatasetIDs(datasetIDs)
	datasetsUsed := buildDatasetsUsed(h.DatasetClient, h.DatasetBaseURL, datasetIDs, registries)

	out := blocksResponse{
		ScopeType:     "country",
		ScopeValue:    cc,
		DatasetsUsed:  datasetsUsed,
		AddressFamily: respAF,
		Count:         len(blockItems),
		Total:         total,
		Limit:         limit,
		Offset:        offset,
		HasMore:       offset+len(blockItems) < int(total),
		Items:         blockItems,
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

// buildDatasetsUsed fills registry from store (scope_imports); serial from dataset-service (best-effort).
func buildDatasetsUsed(client *http.Client, baseURL string, datasetIDs []uuid.UUID, registries map[uuid.UUID]string) []datasetUsedItem {
	out := make([]datasetUsedItem, 0, len(datasetIDs))
	for _, id := range datasetIDs {
		item := datasetUsedItem{DatasetID: id, Registry: registries[id]}
		if client != nil && baseURL != "" {
			if meta := fetchDatasetMetaWithTimeout(client, baseURL, id); meta != nil {
				item.Serial = strconv.FormatInt(meta.Serial, 10)
				if item.Registry == "" {
					item.Registry = meta.Registry
				}
			}
		}
		out = append(out, item)
	}
	return out
}

func fetchDatasetMetaWithTimeout(client *http.Client, baseURL string, id uuid.UUID) *datasetMetaResponse {
	ctx, cancel := context.WithTimeout(context.Background(), datasetMetaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/datasets/"+id.String(), nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()
	var meta datasetMetaResponse
	if json.NewDecoder(resp.Body).Decode(&meta) != nil {
		return nil
	}
	return &meta
}

func writeJSONError(w http.ResponseWriter, status int, errCode string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": errCode})
}
