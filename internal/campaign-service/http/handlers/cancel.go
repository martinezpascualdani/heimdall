package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/domain"
)

// CancelHandler handles POST /v1/runs/{id}/cancel.
type CancelHandler struct {
	RunStore   RunStore
	RunUpdater RunUpdater
}

// ServeHTTP marks the run as canceled (source of truth). Effective respect depends on data plane.
func (h *CancelHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	run, err := h.RunStore.GetCampaignRunByID(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		writeJSONError(w, "run not found", http.StatusNotFound)
		return
	}
	// Only allow cancel if pending or dispatching; if already dispatched, still mark canceled so workers can respect it
	if run.Status != domain.StatusPending && run.Status != domain.StatusDispatching && run.Status != domain.StatusDispatched {
		writeJSONError(w, "run cannot be canceled in current state", http.StatusBadRequest)
		return
	}
	run.Status = domain.StatusCanceled
	if err := h.RunUpdater.UpdateCampaignRun(run); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, runToResp(run))
}
