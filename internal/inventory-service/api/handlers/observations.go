package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/domain"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/storage"
)

// ObservationsHandler handles GET /v1/observations.
type ObservationsHandler struct {
	Store *storage.PostgresStore
}

// List returns paginated observations with filters: execution_id, job_id, run_id, campaign_id, target_id, asset_id, from_time, to_time.
func (h *ObservationsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	var executionID, jobID, runID, campaignID, targetID, assetID *uuid.UUID
	for _, q := range []struct {
		name string
		out  **uuid.UUID
	}{
		{"execution_id", &executionID},
		{"job_id", &jobID},
		{"run_id", &runID},
		{"campaign_id", &campaignID},
		{"target_id", &targetID},
		{"asset_id", &assetID},
	} {
		if s := r.URL.Query().Get(q.name); s != "" {
			if u, err := uuid.Parse(s); err == nil {
				*q.out = &u
			}
		}
	}
	var fromTime, toTime *time.Time
	if s := r.URL.Query().Get("from_time"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			fromTime = &t
		}
	}
	if s := r.URL.Query().Get("to_time"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			toTime = &t
		}
	}
	list, total, err := h.Store.ListObservations(executionID, jobID, runID, campaignID, targetID, assetID, fromTime, toTime, limit, offset)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, o := range list {
		items = append(items, observationToMap(o))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func observationToMap(o *domain.Observation) map[string]interface{} {
	if o == nil {
		return nil
	}
	m := map[string]interface{}{
		"id":           o.ID,
		"execution_id": o.ExecutionID,
		"job_id":       o.JobID,
		"asset_id":     o.AssetID,
		"exposure_id":  o.ExposureID,
		"observed_at":  o.ObservedAt,
		"created_at":   o.CreatedAt,
	}
	if o.RunID != uuid.Nil {
		m["run_id"] = o.RunID
	}
	if o.CampaignID != uuid.Nil {
		m["campaign_id"] = o.CampaignID
	}
	if o.TargetID != uuid.Nil {
		m["target_id"] = o.TargetID
	}
	if o.TargetMaterializationID != uuid.Nil {
		m["target_materialization_id"] = o.TargetMaterializationID
	}
	if o.ScanProfileSlug != "" {
		m["scan_profile_slug"] = o.ScanProfileSlug
	}
	return m
}
