package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/storage"
)

// ExposuresHandler handles GET /v1/exposures and GET /v1/assets/{id}/exposures.
type ExposuresHandler struct {
	Store *storage.PostgresStore
}

// List returns paginated exposures with filters: asset_id, asset_type, exposure_type, campaign_id, target_id.
func (h *ExposuresHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	var assetID *uuid.UUID
	if s := r.URL.Query().Get("asset_id"); s != "" {
		if u, err := uuid.Parse(s); err == nil {
			assetID = &u
		}
	}
	assetType := r.URL.Query().Get("asset_type")
	exposureType := r.URL.Query().Get("exposure_type")
	var campaignID, targetID *uuid.UUID
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
	list, total, err := h.Store.ListExposures(assetID, assetType, exposureType, campaignID, targetID, limit, offset)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, e := range list {
		items = append(items, exposureToMap(e))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// ListByAssetID returns exposures for a single asset (GET /v1/assets/{id}/exposures).
func (h *ExposuresHandler) ListByAssetID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	assetID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid asset id", http.StatusBadRequest)
		return
	}
	list, err := h.Store.ListExposuresByAssetID(assetID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, e := range list {
		items = append(items, exposureToMap(e))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}
