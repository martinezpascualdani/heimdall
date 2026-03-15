package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/martinezpascualdani/heimdall/internal/execution-service/domain"
)

// WorkersHandler handles worker registration, list, get, heartbeat, and list jobs.
type WorkersHandler struct {
	Store interface {
		GetWorkerByID(uuid.UUID) (*domain.Worker, error)
		ListWorkers(limit, offset int, status string) ([]*domain.Worker, int, error)
		ListJobsByAssignedWorker(uuid.UUID, int) ([]*domain.ExecutionJob, error)
		CreateWorker(*domain.Worker) error
		UpdateWorkerHeartbeat(uuid.UUID, time.Time) error
		UpdateWorker(uuid.UUID, []string, int) error
		CountActiveJobsByWorker(uuid.UUID) (int, error)
	}
}

// RegisterRequest is the body for POST /v1/workers.
type RegisterRequest struct {
	Name           string   `json:"name"`
	Region         string   `json:"region"`
	Version        string   `json:"version"`
	Capabilities   []string `json:"capabilities"`
	MaxConcurrency int      `json:"max_concurrency"`
}

// Register creates or updates a worker and returns worker_id.
func (h *WorkersHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.MaxConcurrency <= 0 {
		req.MaxConcurrency = 1
	}
	wkr := &domain.Worker{
		Name:           req.Name,
		Region:         req.Region,
		Version:        req.Version,
		Capabilities:   req.Capabilities,
		MaxConcurrency: req.MaxConcurrency,
		Status:         domain.WorkerStatusOnline,
	}
	now := time.Now()
	wkr.LastHeartbeatAt = &now
	if err := h.Store.CreateWorker(wkr); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"worker_id": wkr.ID,
		"name":      wkr.Name,
		"region":    wkr.Region,
		"version":   wkr.Version,
		"capabilities": wkr.Capabilities,
		"max_concurrency": wkr.MaxConcurrency,
		"status": wkr.Status,
	})
}

// ListJobs returns jobs currently assigned to this worker (assigned or running). For debugging.
func (h *WorkersHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	workerID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid worker id", http.StatusBadRequest)
		return
	}
	limit, _ := parseLimitOffset(r, defaultLimit, maxLimit)
	jobs, err := h.Store.ListJobsByAssignedWorker(workerID, limit)
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
		"total":  len(items),
		"limit":  limit,
	})
}

// HeartbeatRequest can include capabilities for PATCH /v1/workers/{id}.
type HeartbeatRequest struct {
	Capabilities   []string `json:"capabilities,omitempty"`
	MaxConcurrency *int    `json:"max_concurrency,omitempty"`
}

// Heartbeat updates last_heartbeat_at and optionally capabilities/max_concurrency.
func (h *WorkersHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	if idStr == "" {
		writeJSONError(w, "missing worker id", http.StatusBadRequest)
		return
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid worker id", http.StatusBadRequest)
		return
	}
	wkr, err := h.Store.GetWorkerByID(id)
	if err != nil || wkr == nil {
		writeJSONError(w, "worker not found", http.StatusNotFound)
		return
	}
	now := time.Now()
	if err := h.Store.UpdateWorkerHeartbeat(id, now); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var req HeartbeatRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if len(req.Capabilities) > 0 || req.MaxConcurrency != nil {
		capabilities := wkr.Capabilities
		if len(req.Capabilities) > 0 {
			capabilities = req.Capabilities
		}
		mc := wkr.MaxConcurrency
		if req.MaxConcurrency != nil && *req.MaxConcurrency > 0 {
			mc = *req.MaxConcurrency
		}
		_ = h.Store.UpdateWorker(id, capabilities, mc)
	}
	load, _ := h.Store.CountActiveJobsByWorker(id)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"worker_id": id,
		"last_heartbeat_at": now,
		"current_load": load,
	})
}

// HeartbeatTimeoutForDisplay is used to show "offline" in GET when last heartbeat is older (no DB write).
const HeartbeatTimeoutForDisplay = 2 * time.Minute

// List returns workers with optional status filter; pagination limit/offset.
func (h *WorkersHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset := parseLimitOffset(r, defaultLimit, maxLimit)
	status := r.URL.Query().Get("status")
	list, total, err := h.Store.ListWorkers(limit, offset, status)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, w := range list {
		load, _ := h.Store.CountActiveJobsByWorker(w.ID)
		effectiveStatus := w.Status
		if w.LastHeartbeatAt == nil || time.Since(*w.LastHeartbeatAt) > HeartbeatTimeoutForDisplay {
			effectiveStatus = domain.WorkerStatusOffline
		}
		m := map[string]interface{}{
			"id":                  w.ID,
			"name":                w.Name,
			"region":              w.Region,
			"version":             w.Version,
			"capabilities":        w.Capabilities,
			"status":              effectiveStatus,
			"last_heartbeat_at":   w.LastHeartbeatAt,
			"max_concurrency":     w.MaxConcurrency,
			"current_load":        load,
			"created_at":          w.CreatedAt,
			"updated_at":          w.UpdatedAt,
		}
		items = append(items, m)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Get returns one worker by ID (with derived current_load). Status in response is "effective":
// if last_heartbeat_at is older than HeartbeatTimeoutForDisplay, response status is "offline".
func (h *WorkersHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSONError(w, "invalid worker id", http.StatusBadRequest)
		return
	}
	wkr, err := h.Store.GetWorkerByID(id)
	if err != nil || wkr == nil {
		writeJSONError(w, "worker not found", http.StatusNotFound)
		return
	}
	load, _ := h.Store.CountActiveJobsByWorker(id)
	status := wkr.Status
	if wkr.LastHeartbeatAt == nil || time.Since(*wkr.LastHeartbeatAt) > HeartbeatTimeoutForDisplay {
		status = domain.WorkerStatusOffline
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                 wkr.ID,
		"name":               wkr.Name,
		"region":             wkr.Region,
		"version":            wkr.Version,
		"capabilities":       wkr.Capabilities,
		"status":             status,
		"last_heartbeat_at":  wkr.LastHeartbeatAt,
		"max_concurrency":    wkr.MaxConcurrency,
		"current_load":       load,
		"created_at":         wkr.CreatedAt,
		"updated_at":         wkr.UpdatedAt,
	})
}
