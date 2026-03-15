package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/target-service/domain"
)

// MaterializeStore is the storage interface for materialize and materializations.
type MaterializeStore interface {
	GetTargetByID(uuid.UUID) (*domain.Target, error)
	ListRulesByTargetID(uuid.UUID) ([]domain.TargetRule, error)
	CreateMaterialization(*domain.TargetMaterialization) error
	UpdateMaterialization(*domain.TargetMaterialization) error
	GetMaterializationByID(uuid.UUID) (*domain.TargetMaterialization, error)
	ListMaterializations(uuid.UUID, int, int) ([]*domain.TargetMaterialization, error)
	InsertTargetEntries(uuid.UUID, []string) error
	DeleteEntriesForMaterialization(uuid.UUID) error
	ListPrefixes(uuid.UUID, int, int) ([]string, error)
	CountPrefixes(uuid.UUID) (int, error)
	GetAllPrefixesForMaterialization(uuid.UUID) ([]string, error)
	GetMaterializationTargetID(uuid.UUID) (uuid.UUID, error)
}

// Materializer runs the materialization.
type Materializer interface {
	Run(ctx context.Context, targetID uuid.UUID, rules []domain.TargetRule) (materializationID uuid.UUID, err error)
}

// MaterializeHandler handles POST /v1/targets/{id}/materialize.
type MaterializeHandler struct {
	Store        MaterializeStore
	Materializer Materializer
}

// ServeHTTP handles POST /v1/targets/{id}/materialize (synchronous in v1).
func (h *MaterializeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	t, err := h.Store.GetTargetByID(id)
	if err != nil || t == nil {
		writeJSONError(w, "target not found", http.StatusNotFound)
		return
	}
	rules, err := h.Store.ListRulesByTargetID(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	mid, err := h.Materializer.Run(r.Context(), id, rules)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	m, _ := h.Store.GetMaterializationByID(mid)
	resp := map[string]interface{}{
		"materialization_id": mid,
		"status":             domain.MaterializationStatusCompleted,
	}
	if m != nil {
		resp["total_prefix_count"] = m.TotalPrefixCount
		resp["materialized_at"] = m.MaterializedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
