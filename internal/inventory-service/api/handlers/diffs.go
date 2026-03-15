package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/storage"
)

// DiffsHandler handles GET /v1/diffs/executions.
// Diff compares observation sets (pairs asset_id, exposure_id) per execution from the
// observations table only; it does not use last_seen or current state, so results are exact.
type DiffsHandler struct {
	Store *storage.PostgresStore
}

// ExecutionsDiff returns diff between two executions: assets_new, exposures_new, exposures_gone.
func (h *DiffsHandler) ExecutionsDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fromStr := r.URL.Query().Get("from_execution_id")
	toStr := r.URL.Query().Get("to_execution_id")
	if fromStr == "" || toStr == "" {
		writeJSONError(w, "from_execution_id and to_execution_id are required", http.StatusBadRequest)
		return
	}
	fromID, err := uuid.Parse(fromStr)
	if err != nil {
		writeJSONError(w, "invalid from_execution_id", http.StatusBadRequest)
		return
	}
	toID, err := uuid.Parse(toStr)
	if err != nil {
		writeJSONError(w, "invalid to_execution_id", http.StatusBadRequest)
		return
	}
	pairsFrom, err := h.Store.ListObservationPairsByExecution(fromID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pairsTo, err := h.Store.ListObservationPairsByExecution(toID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setFrom := make(map[string]struct{})
	assetIDsFrom := make(map[uuid.UUID]struct{})
	for _, p := range pairsFrom {
		key := p.AssetID.String() + "\x00" + p.ExposureID.String()
		setFrom[key] = struct{}{}
		assetIDsFrom[p.AssetID] = struct{}{}
	}
	setTo := make(map[string]struct{})
	assetIDsTo := make(map[uuid.UUID]struct{})
	for _, p := range pairsTo {
		key := p.AssetID.String() + "\x00" + p.ExposureID.String()
		setTo[key] = struct{}{}
		assetIDsTo[p.AssetID] = struct{}{}
	}
	var exposuresNew, exposuresGone, exposuresUnchanged []map[string]interface{}
	seenAssetNew := make(map[uuid.UUID]struct{})
	for _, p := range pairsTo {
		key := p.AssetID.String() + "\x00" + p.ExposureID.String()
		if _, in := setFrom[key]; !in {
			e, _ := h.Store.GetExposureByID(p.ExposureID)
			if e != nil {
				exposuresNew = append(exposuresNew, exposureToMap(e))
				seenAssetNew[p.AssetID] = struct{}{}
			}
		} else {
			e, _ := h.Store.GetExposureByID(p.ExposureID)
			if e != nil {
				exposuresUnchanged = append(exposuresUnchanged, exposureToMap(e))
			}
		}
	}
	for _, p := range pairsFrom {
		key := p.AssetID.String() + "\x00" + p.ExposureID.String()
		if _, in := setTo[key]; !in {
			e, _ := h.Store.GetExposureByID(p.ExposureID)
			if e != nil {
				exposuresGone = append(exposuresGone, exposureToMap(e))
			}
		}
	}
	var assetsNew []map[string]interface{}
	for aid := range seenAssetNew {
		if _, inFrom := assetIDsFrom[aid]; !inFrom {
			a, _ := h.Store.GetAsset(aid)
			if a != nil {
				assetsNew = append(assetsNew, assetToMap(a))
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"from_execution_id":  fromStr,
		"to_execution_id":    toStr,
		"assets_new":         assetsNew,
		"exposures_new":      exposuresNew,
		"exposures_gone":     exposuresGone,
		"exposures_unchanged": exposuresUnchanged,
	})
}
