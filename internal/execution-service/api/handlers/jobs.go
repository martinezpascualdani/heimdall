package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/martinezpascualdani/heimdall/internal/execution-service/domain"
)

// JobsHandler handles job claim, complete, fail, renew.
// Inventory ingest is via Redis (outbox-publisher -> stream -> inventory-service consumer); no HTTP notifier.
type JobsHandler struct {
	Store interface {
		GetWorkerByID(uuid.UUID) (*domain.Worker, error)
		GetJobByID(uuid.UUID) (*domain.ExecutionJob, error)
		GetExecutionByID(uuid.UUID) (*domain.Execution, error)
		ClaimJob(workerID uuid.UUID, maxConcurrency int, scanProfileSlug string, leaseDuration time.Duration) (*domain.ExecutionJob, error)
		JobComplete(jobID, workerID uuid.UUID, leaseID string, resultSummary json.RawMessage) error
		JobFail(jobID, workerID uuid.UUID, leaseID string, errorMessage string) error
		JobRenew(jobID, workerID uuid.UUID, leaseID string, newExpiresAt time.Time) (bool, error)
	}
	LeaseDuration time.Duration
}

// ClaimRequest is the body for POST /v1/jobs/claim.
type ClaimRequest struct {
	WorkerID     string   `json:"worker_id"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// Claim assigns a pending job to the worker and returns it with lease_id and lease_expires_at.
func (h *JobsHandler) Claim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	workerID, err := uuid.Parse(req.WorkerID)
	if err != nil {
		writeJSONError(w, "invalid worker_id", http.StatusBadRequest)
		return
	}
	wkr, err := h.Store.GetWorkerByID(workerID)
	if err != nil || wkr == nil {
		writeJSONError(w, "worker not found", http.StatusNotFound)
		return
	}
	leaseDur := h.LeaseDuration
	if leaseDur <= 0 {
		leaseDur = 5 * time.Minute
	}
	// Claim a job matching any of the worker's capabilities; if none, use first capability or empty (store will match running executions).
	slug := ""
	if len(req.Capabilities) > 0 {
		slug = req.Capabilities[0]
	} else if len(wkr.Capabilities) > 0 {
		slug = wkr.Capabilities[0]
	}
	// Try each capability until we claim one
	for _, s := range wkr.Capabilities {
		j, err := h.Store.ClaimJob(workerID, wkr.MaxConcurrency, s, leaseDur)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if j != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"id":                 j.ID,
				"execution_id":       j.ExecutionID,
				"payload":            j.Payload,
				"status":             j.Status,
				"lease_id":           j.LeaseID,
				"lease_expires_at":   j.LeaseExpiresAt,
				"attempt":            j.Attempt,
				"max_attempts":       j.MaxAttempts,
			})
			return
		}
	}
	if slug != "" {
		j, _ := h.Store.ClaimJob(workerID, wkr.MaxConcurrency, slug, leaseDur)
		if j != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"id":                 j.ID,
				"execution_id":       j.ExecutionID,
				"payload":            j.Payload,
				"status":             j.Status,
				"lease_id":           j.LeaseID,
				"lease_expires_at":   j.LeaseExpiresAt,
				"attempt":            j.Attempt,
				"max_attempts":       j.MaxAttempts,
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"job": nil})
}

// CompleteRequest is the body for POST /v1/jobs/{id}/complete.
type CompleteRequest struct {
	WorkerID      string          `json:"worker_id"`
	LeaseID       string          `json:"lease_id"`
	ResultSummary json.RawMessage `json:"result_summary"`
}

// Complete marks the job as completed.
func (h *JobsHandler) Complete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	jobID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid job id", http.StatusBadRequest)
		return
	}
	var req CompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.WorkerID == "" {
		req.WorkerID = r.URL.Query().Get("worker_id")
	}
	if req.WorkerID == "" {
		writeJSONError(w, "worker_id required", http.StatusBadRequest)
		return
	}
	workerID, err := uuid.Parse(req.WorkerID)
	if err != nil {
		writeJSONError(w, "invalid worker_id", http.StatusBadRequest)
		return
	}
	if err := h.Store.JobComplete(jobID, workerID, req.LeaseID, req.ResultSummary); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("execution-service: job job_id=%s completed by worker_id=%s", jobID, workerID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

// FailRequest is the body for POST /v1/jobs/{id}/fail.
type FailRequest struct {
	WorkerID     string `json:"worker_id"`
	LeaseID      string `json:"lease_id"`
	ErrorMessage string `json:"error_message"`
}

// Fail marks the job as failed.
func (h *JobsHandler) Fail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	jobID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid job id", http.StatusBadRequest)
		return
	}
	var req FailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.WorkerID == "" {
		req.WorkerID = r.URL.Query().Get("worker_id")
	}
	if req.WorkerID == "" {
		writeJSONError(w, "worker_id required", http.StatusBadRequest)
		return
	}
	workerID, err := uuid.Parse(req.WorkerID)
	if err != nil {
		writeJSONError(w, "invalid worker_id", http.StatusBadRequest)
		return
	}
	if err := h.Store.JobFail(jobID, workerID, req.LeaseID, req.ErrorMessage); err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("execution-service: job job_id=%s failed (worker_id=%s): %s", jobID, workerID, req.ErrorMessage)
	writeJSON(w, http.StatusOK, map[string]string{"status": "failed"})
}

// RenewRequest is the body for POST /v1/jobs/{id}/renew.
type RenewRequest struct {
	WorkerID string `json:"worker_id"`
	LeaseID  string `json:"lease_id"`
}

// Renew extends the job lease.
func (h *JobsHandler) Renew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	jobID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid job id", http.StatusBadRequest)
		return
	}
	var req RenewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.WorkerID == "" {
		req.WorkerID = r.URL.Query().Get("worker_id")
	}
	if req.WorkerID == "" {
		writeJSONError(w, "worker_id required", http.StatusBadRequest)
		return
	}
	workerID, err := uuid.Parse(req.WorkerID)
	if err != nil {
		writeJSONError(w, "invalid worker_id", http.StatusBadRequest)
		return
	}
	leaseDur := h.LeaseDuration
	if leaseDur <= 0 {
		leaseDur = 5 * time.Minute
	}
	ok, err := h.Store.JobRenew(jobID, workerID, req.LeaseID, time.Now().Add(leaseDur))
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		writeJSONError(w, "lease not found or expired", http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "renewed"})
}
