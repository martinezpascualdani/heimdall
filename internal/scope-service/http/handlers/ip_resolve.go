package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/ipresolver"
)

// Resolver is the interface used for IP resolution (allows mocking in tests).
type Resolver interface {
	Resolve(ctx context.Context, ipStr string, datasetID *uuid.UUID) (*ipresolver.ResolveResult, error)
}

// IPResolveHandler handles GET /v1/scopes/by-ip/{ip}.
type IPResolveHandler struct {
	Resolver       Resolver
	DatasetBaseURL string
	DatasetClient  *http.Client
}

// datasetMeta is the subset of dataset-service response we need for enrichment.
type datasetMeta struct {
	Registry string `json:"registry"`
	Serial   int64  `json:"serial"`
}

// Response is the JSON body for a successful IP resolve.
type Response struct {
	IP         string    `json:"ip"`
	ScopeType  string    `json:"scope_type"`
	ScopeValue string    `json:"scope_value"`
	DatasetID  uuid.UUID `json:"dataset_id"`
	Registry   string    `json:"registry,omitempty"`
	Serial     int64     `json:"serial,omitempty"`
}

// ServeHTTP implements http.Handler.
func (h *IPResolveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ipStr := r.PathValue("ip")
	if ipStr == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_ip"})
		return
	}

	var datasetID *uuid.UUID
	if idStr := r.URL.Query().Get("dataset_id"); idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid dataset_id"})
			return
		}
		datasetID = &id
	}

	result, err := h.Resolver.Resolve(r.Context(), ipStr, datasetID)
	if err != nil {
		switch {
		case errors.Is(err, ipresolver.ErrInvalidIP):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_ip"})
			return
		case errors.Is(err, ipresolver.ErrNoDataset):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"error": "no_dataset_available"})
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if result == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "ip_not_found"})
		return
	}

	resp := Response{
		IP:         result.IP,
		ScopeType:  result.ScopeType,
		ScopeValue: result.ScopeValue,
		DatasetID:  result.DatasetID,
	}
	if h.DatasetBaseURL != "" && h.DatasetClient != nil {
		if meta := fetchDatasetMeta(h.DatasetClient, h.DatasetBaseURL, result.DatasetID); meta != nil {
			resp.Registry = meta.Registry
			resp.Serial = meta.Serial
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func fetchDatasetMeta(client *http.Client, baseURL string, id uuid.UUID) *datasetMeta {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/datasets/"+id.String(), nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()
	var meta datasetMeta
	if json.NewDecoder(resp.Body).Decode(&meta) != nil {
		return nil
	}
	return &meta
}
