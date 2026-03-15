package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/target-service/domain"
)

// MaterializationsHandler handles GET /v1/targets/{id}/materializations and GET /v1/targets/{id}/materializations/{mid}.
type MaterializationsHandler struct {
	Store MaterializeStore
}

// List handles GET /v1/targets/{id}/materializations (paginated).
func (h *MaterializationsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	list, err := h.Store.ListMaterializations(id, limit, offset)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, m := range list {
		items = append(items, materializationToResp(m))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items": items,
		"count": len(items),
	})
}

// Get handles GET /v1/targets/{id}/materializations/{mid} (metadata only, no prefixes).
func (h *MaterializationsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	mid, err := uuid.Parse(r.PathValue("mid"))
	if err != nil {
		writeJSONError(w, "invalid materialization id", http.StatusBadRequest)
		return
	}
	m, err := h.Store.GetMaterializationByID(mid)
	if err != nil || m == nil {
		writeJSONError(w, "materialization not found", http.StatusNotFound)
		return
	}
	if m.TargetID != id {
		writeJSONError(w, "materialization not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(materializationToResp(m))
}

// PrefixesHandler handles GET /v1/targets/{id}/materializations/{mid}/prefixes (paginated).
type PrefixesHandler struct {
	Store MaterializeStore
}

// ServeHTTP handles GET /v1/targets/{id}/materializations/{mid}/prefixes.
func (h *PrefixesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	mid, err := uuid.Parse(r.PathValue("mid"))
	if err != nil {
		writeJSONError(w, "invalid materialization id", http.StatusBadRequest)
		return
	}
	m, err := h.Store.GetMaterializationByID(mid)
	if err != nil || m == nil || m.TargetID != id {
		writeJSONError(w, "materialization not found", http.StatusNotFound)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	prefixes, err := h.Store.ListPrefixes(mid, limit, offset)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	total, _ := h.Store.CountPrefixes(mid)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"materialization_id": mid,
		"count":              len(prefixes),
		"total":              total,
		"limit":              limit,
		"offset":             offset,
		"has_more":           offset+len(prefixes) < total,
		"items":              prefixes,
	})
}

func materializationToResp(m *domain.TargetMaterialization) map[string]interface{} {
	return map[string]interface{}{
		"id":                   m.ID,
		"target_id":            m.TargetID,
		"materialized_at":      m.MaterializedAt.Format("2006-01-02T15:04:05Z07:00"),
		"total_prefix_count":   m.TotalPrefixCount,
		"status":               m.Status,
		"error_message":        m.ErrorMessage,
		"scope_snapshot_ref":   m.ScopeSnapshotRef,
		"routing_snapshot_ref": m.RoutingSnapshotRef,
	}
}
