package handlers

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/martinezpascualdani/heimdall/internal/execution-service/domain"
)

// ExecutionsHandler handles list and get executions and their jobs.
type ExecutionsHandler struct {
	Store interface {
		GetExecutionByID(uuid.UUID) (*domain.Execution, error)
		ListExecutions(limit, offset int, runID, campaignID *uuid.UUID, status string) ([]*domain.Execution, int, error)
		ListJobsByExecution(executionID uuid.UUID, limit, offset int) ([]*domain.ExecutionJob, int, error)
		RequeueFailedJobsForExecution(executionID uuid.UUID) (requeued int, err error)
		CancelExecution(executionID uuid.UUID) (canceled int, err error)
	}
}

// List returns executions with optional filters run_id, campaign_id, status; pagination limit/offset.
func (h *ExecutionsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	var runID, campaignID *uuid.UUID
	if s := r.URL.Query().Get("run_id"); s != "" {
		if u, err := uuid.Parse(s); err == nil {
			runID = &u
		}
	}
	if s := r.URL.Query().Get("campaign_id"); s != "" {
		if u, err := uuid.Parse(s); err == nil {
			campaignID = &u
		}
	}
	status := r.URL.Query().Get("status")
	list, total, err := h.Store.ListExecutions(limit, offset, runID, campaignID, status)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, e := range list {
		items = append(items, executionToMap(e))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Get returns one execution by ID.
func (h *ExecutionsHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid execution id", http.StatusBadRequest)
		return
	}
	e, err := h.Store.GetExecutionByID(id)
	if err != nil || e == nil {
		writeJSONError(w, "execution not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, executionToMap(e))
}

// ListJobs returns jobs for an execution (paginated).
func (h *ExecutionsHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	execID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid execution id", http.StatusBadRequest)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	jobs, total, err := h.Store.ListJobsByExecution(execID, limit, offset)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(jobs))
	for _, j := range jobs {
		items = append(items, JobToMap(j))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Requeue puts all failed jobs of the execution (with attempt < max_attempts) back to pending so workers can claim them again.
func (h *ExecutionsHandler) Requeue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	execID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid execution id", http.StatusBadRequest)
		return
	}
	e, err := h.Store.GetExecutionByID(execID)
	if err != nil || e == nil {
		writeJSONError(w, "execution not found", http.StatusNotFound)
		return
	}
	requeued, err := h.Store.RequeueFailedJobsForExecution(execID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"requeued": requeued})
}

// Cancel sets execution and all its pending/assigned/running jobs to canceled so workers stop claiming them.
func (h *ExecutionsHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	execID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid execution id", http.StatusBadRequest)
		return
	}
	e, err := h.Store.GetExecutionByID(execID)
	if err != nil || e == nil {
		writeJSONError(w, "execution not found", http.StatusNotFound)
		return
	}
	if e.Status == domain.ExecutionStatusCanceled || e.Status == domain.ExecutionStatusCompleted {
		writeJSONError(w, "execution already in terminal state", http.StatusBadRequest)
		return
	}
	canceled, err := h.Store.CancelExecution(execID)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"canceled": canceled, "status": "canceled"})
}

func executionToMap(e *domain.Execution) map[string]interface{} {
	m := map[string]interface{}{
		"id":                         e.ID,
		"run_id":                     e.RunID,
		"campaign_id":                e.CampaignID,
		"target_id":                  e.TargetID,
		"target_materialization_id":  e.TargetMaterializationID,
		"scan_profile_slug":          e.ScanProfileSlug,
		"status":                     e.Status,
		"total_jobs":                 e.TotalJobs,
		"completed_jobs":             e.CompletedJobs,
		"failed_jobs":                e.FailedJobs,
		"created_at":                 e.CreatedAt,
		"updated_at":                 e.UpdatedAt,
		"error_summary":              e.ErrorSummary,
	}
	if e.ScanProfileConfig != nil {
		m["scan_profile_config"] = e.ScanProfileConfig
	}
	if e.CompletedAt != nil {
		m["completed_at"] = e.CompletedAt
	}
	return m
}

