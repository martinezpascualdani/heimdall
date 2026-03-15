package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/martinezpascualdani/heimdall/internal/execution-service/domain"
)

const defaultLimit = 100
const maxLimit = 10_000

func parseLimitOffset(r *http.Request, defaultL, maxL int) (limit, offset int) {
	limit = defaultL
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			limit = n
			if limit > maxL {
				limit = maxL
			}
		}
	}
	offset = 0
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// JobToMap converts ExecutionJob to a map for JSON response (shared by executions and workers handlers).
func JobToMap(j *domain.ExecutionJob) map[string]interface{} {
	if j == nil {
		return nil
	}
	m := map[string]interface{}{
		"id":            j.ID,
		"execution_id":  j.ExecutionID,
		"payload":       j.Payload,
		"status":        j.Status,
		"attempt":       j.Attempt,
		"max_attempts":  j.MaxAttempts,
		"error_message": j.ErrorMessage,
		"created_at":    j.CreatedAt,
		"updated_at":    j.UpdatedAt,
	}
	if j.AssignedWorkerID != nil {
		m["assigned_worker_id"] = j.AssignedWorkerID
	}
	if j.LeaseExpiresAt != nil {
		m["lease_expires_at"] = j.LeaseExpiresAt
	}
	if j.LeaseID != "" {
		m["lease_id"] = j.LeaseID
	}
	if j.ResultSummary != nil {
		m["result_summary"] = j.ResultSummary
	}
	if j.StartedAt != nil {
		m["started_at"] = j.StartedAt
	}
	if j.CompletedAt != nil {
		m["completed_at"] = j.CompletedAt
	}
	return m
}
