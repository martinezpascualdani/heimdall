package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/campaign-service/domain"
)

// RunStore is the storage interface for runs.
type RunStore interface {
	GetCampaignRunByID(uuid.UUID) (*domain.CampaignRun, error)
	ListCampaignRuns(campaignID uuid.UUID, limit, offset int) ([]*domain.CampaignRun, int, error)
}

// RunsHandler handles GET runs (list by campaign, get one).
type RunsHandler struct {
	Store RunStore
}

// ListByCampaign handles GET /v1/campaigns/{id}/runs.
func (h *RunsHandler) ListByCampaign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	campaignID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	list, total, err := h.Store.ListCampaignRuns(campaignID, limit, offset)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, run := range list {
		items = append(items, runToResp(run))
	}
	hasMore := offset+len(items) < total
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":    items,
		"count":    len(items),
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": hasMore,
	})
}

// GetRun handles GET /v1/runs/{id}.
func (h *RunsHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, "invalid id", http.StatusBadRequest)
		return
	}
	run, err := h.Store.GetCampaignRunByID(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		writeJSONError(w, "run not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, runToResp(run))
}

func runToResp(r *domain.CampaignRun) map[string]interface{} {
	resp := map[string]interface{}{
		"id":                          r.ID,
		"campaign_id":                 r.CampaignID,
		"target_id":                   r.TargetID,
		"target_materialization_id":  r.TargetMaterializationID,
		"scan_profile_id":             r.ScanProfileID,
		"scan_profile_slug":           r.ScanProfileSlug,
		"scan_profile_config_snapshot": r.ScanProfileConfigSnapshot,
		"status":                      r.Status,
		"created_at":                  r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"dispatch_ref":                r.DispatchRef,
		"error_message":               r.ErrorMessage,
		"stats":                       r.Stats,
	}
	if r.StartedAt != nil {
		resp["started_at"] = r.StartedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if r.CompletedAt != nil {
		resp["completed_at"] = r.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if r.DispatchedAt != nil {
		resp["dispatched_at"] = r.DispatchedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}
