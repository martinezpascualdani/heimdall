package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/storage"
)

// IngestHandler handles POST /v1/ingest/job-completed.
type IngestHandler struct {
	Store *storage.PostgresStore
}

// JobCompletedRequest is the contract for job-completed ingesta (explicit payload, not result_summary).
type JobCompletedRequest struct {
	ExecutionID             string    `json:"execution_id"`
	JobID                   string    `json:"job_id"`
	RunID                   string    `json:"run_id"`
	CampaignID              string    `json:"campaign_id"`
	TargetID                string    `json:"target_id"`
	TargetMaterializationID string    `json:"target_materialization_id"`
	ScanProfileSlug         string    `json:"scan_profile_slug"`
	ObservedAt              time.Time `json:"observed_at"`
	Observations            []struct {
		IP     string `json:"ip"`
		Port   int    `json:"port"`
		Status string `json:"status"`
	} `json:"observations"`
}

// JobCompleted handles POST /v1/ingest/job-completed.
func (h *IngestHandler) JobCompleted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req JobCompletedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	executionID, err := uuid.Parse(req.ExecutionID)
	if err != nil {
		writeJSONError(w, "invalid execution_id", http.StatusBadRequest)
		return
	}
	jobID, err := uuid.Parse(req.JobID)
	if err != nil {
		writeJSONError(w, "invalid job_id", http.StatusBadRequest)
		return
	}
	if req.ObservedAt.IsZero() {
		writeJSONError(w, "observed_at is required", http.StatusBadRequest)
		return
	}
	var runID, campaignID, targetID, targetMatID uuid.UUID
	if req.RunID != "" {
		runID, _ = uuid.Parse(req.RunID)
	}
	if req.CampaignID != "" {
		campaignID, _ = uuid.Parse(req.CampaignID)
	}
	if req.TargetID != "" {
		targetID, _ = uuid.Parse(req.TargetID)
	}
	if req.TargetMaterializationID != "" {
		targetMatID, _ = uuid.Parse(req.TargetMaterializationID)
	}
	obs := make([]storage.IngestObservation, len(req.Observations))
	for i, o := range req.Observations {
		obs[i] = storage.IngestObservation{IP: o.IP, Port: o.Port, Status: o.Status}
	}
	err = h.Store.IngestJobCompleted(executionID, jobID, runID, campaignID, targetID, targetMatID, req.ScanProfileSlug, req.ObservedAt, obs)
	if err != nil {
		if errors.Is(err, storage.ErrAlreadyIngested) {
			writeJSONError(w, "job already ingested", http.StatusConflict)
			return
		}
		log.Printf("inventory-service: ingest failed: %v", err)
		writeJSONError(w, "ingest failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "ingested"})
}
