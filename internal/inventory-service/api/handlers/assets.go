package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/domain"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/storage"
)

// AssetsHandler handles GET /v1/assets and GET /v1/assets/{id}.
type AssetsHandler struct {
	Store *storage.PostgresStore
}

// List returns paginated assets with filters: asset_type, campaign_id, target_id, run_id, first_seen_after, last_seen_after.
func (h *AssetsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	assetType := r.URL.Query().Get("asset_type")
	var campaignID, targetID, runID *uuid.UUID
	if s := r.URL.Query().Get("campaign_id"); s != "" {
		if u, err := uuid.Parse(s); err == nil {
			campaignID = &u
		}
	}
	if s := r.URL.Query().Get("target_id"); s != "" {
		if u, err := uuid.Parse(s); err == nil {
			targetID = &u
		}
	}
	if s := r.URL.Query().Get("run_id"); s != "" {
		if u, err := uuid.Parse(s); err == nil {
			runID = &u
		}
	}
	var firstSeenAfter, lastSeenAfter *time.Time
	if s := r.URL.Query().Get("first_seen_after"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			firstSeenAfter = &t
		}
	}
	if s := r.URL.Query().Get("last_seen_after"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			lastSeenAfter = &t
		}
	}
	list, total, err := h.Store.ListAssets(assetType, campaignID, targetID, runID, firstSeenAfter, lastSeenAfter, limit, offset)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, a := range list {
		items = append(items, assetToMap(a))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Get returns one asset by ID with its exposures.
func (h *AssetsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid asset id", http.StatusBadRequest)
		return
	}
	a, err := h.Store.GetAsset(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if a == nil {
		writeJSONError(w, "asset not found", http.StatusNotFound)
		return
	}
	exposures, err := h.Store.ListExposuresByAssetID(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	expMaps := make([]map[string]interface{}, 0, len(exposures))
	for _, e := range exposures {
		expMaps = append(expMaps, exposureToMap(e))
	}
	m := assetToMap(a)
	m["exposures"] = expMaps
	writeJSON(w, http.StatusOK, m)
}

func assetToMap(a *domain.Asset) map[string]interface{} {
	if a == nil {
		return nil
	}
	m := map[string]interface{}{
		"id":                  a.ID,
		"asset_type":          a.AssetType,
		"identity_value":      a.IdentityValue,
		"identity_normalized": a.IdentityNormalized,
		"first_seen_at":       a.FirstSeenAt,
		"last_seen_at":        a.LastSeenAt,
		"created_at":          a.CreatedAt,
		"updated_at":          a.UpdatedAt,
	}
	if len(a.IdentityData) > 0 {
		m["identity_data"] = a.IdentityData
	}
	return m
}

func exposureToMap(e *domain.Exposure) map[string]interface{} {
	if e == nil {
		return nil
	}
	m := map[string]interface{}{
		"id":            e.ID,
		"asset_id":     e.AssetID,
		"exposure_type": e.ExposureType,
		"exposure_key":  e.ExposureKey,
		"first_seen_at": e.FirstSeenAt,
		"last_seen_at":  e.LastSeenAt,
		"created_at":    e.CreatedAt,
		"updated_at":    e.UpdatedAt,
	}
	if e.KeyProtocol != "" {
		m["key_protocol"] = e.KeyProtocol
	}
	if e.KeyPort != nil {
		m["key_port"] = *e.KeyPort
	}
	return m
}
