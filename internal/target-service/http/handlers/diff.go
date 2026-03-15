package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// DiffHandler handles GET /v1/targets/{id}/materializations/diff?from={mid1}&to={mid2}.
type DiffHandler struct {
	Store MaterializeStore
}

// ServeHTTP computes diff between two snapshots of the same target.
func (h *DiffHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		writeJSONError(w, "from and to query params required", http.StatusBadRequest)
		return
	}
	fromID, err := uuid.Parse(fromStr)
	if err != nil {
		writeJSONError(w, "invalid from", http.StatusBadRequest)
		return
	}
	toID, err := uuid.Parse(toStr)
	if err != nil {
		writeJSONError(w, "invalid to", http.StatusBadRequest)
		return
	}
	fromTargetID, err := h.Store.GetMaterializationTargetID(fromID)
	if err != nil || fromTargetID == uuid.Nil {
		writeJSONError(w, "from materialization not found", http.StatusNotFound)
		return
	}
	toTargetID, err := h.Store.GetMaterializationTargetID(toID)
	if err != nil || toTargetID == uuid.Nil {
		writeJSONError(w, "to materialization not found", http.StatusNotFound)
		return
	}
	if fromTargetID != id || toTargetID != id {
		writeJSONError(w, "materializations must belong to this target", http.StatusBadRequest)
		return
	}
	if fromTargetID != toTargetID {
		writeJSONError(w, "cannot diff snapshots from different targets", http.StatusBadRequest)
		return
	}
	fromPrefixes, err := h.Store.GetAllPrefixesForMaterialization(fromID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	toPrefixes, err := h.Store.GetAllPrefixesForMaterialization(toID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fromSet := make(map[string]struct{})
	for _, p := range fromPrefixes {
		fromSet[p] = struct{}{}
	}
	toSet := make(map[string]struct{})
	for _, p := range toPrefixes {
		toSet[p] = struct{}{}
	}
	var added, removed []string
	for p := range toSet {
		if _, ok := fromSet[p]; !ok {
			added = append(added, p)
		}
	}
	for p := range fromSet {
		if _, ok := toSet[p]; !ok {
			removed = append(removed, p)
		}
	}
	fromM, _ := h.Store.GetMaterializationByID(fromID)
	toM, _ := h.Store.GetMaterializationByID(toID)
	resp := map[string]interface{}{
		"from_materialization_id": fromID,
		"to_materialization_id":   toID,
		"added_count":            len(added),
		"removed_count":          len(removed),
		"added":                  added,
		"removed":                removed,
	}
	if fromM != nil {
		resp["from_materialized_at"] = fromM.MaterializedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if toM != nil {
		resp["to_materialized_at"] = toM.MaterializedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
