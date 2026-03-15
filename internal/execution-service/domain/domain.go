package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Worker status.
const (
	WorkerStatusOnline   = "online"
	WorkerStatusOffline  = "offline"
	WorkerStatusDraining = "draining"
	WorkerStatusDisabled = "disabled"
)

// Execution status. planning = creating jobs; running = jobs created, execution active.
const (
	ExecutionStatusPlanning  = "planning"
	ExecutionStatusRunning   = "running"
	ExecutionStatusCompleted = "completed"
	ExecutionStatusFailed    = "failed"
	ExecutionStatusCanceled  = "canceled"
)

// Job status. Requeue is an operation (back to pending), not a persistent state.
const (
	JobStatusPending   = "pending"
	JobStatusAssigned  = "assigned"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusCanceled  = "canceled"
)

// Worker is a registered scan worker. current_load is derived from execution_jobs, not persisted.
type Worker struct {
	ID               uuid.UUID   `json:"id"`
	Name             string      `json:"name"`
	Region           string      `json:"region"`
	Version          string      `json:"version"`
	Capabilities     []string    `json:"capabilities"` // e.g. ["discovery-basic","portscan-basic"]
	Status           string      `json:"status"`
	LastHeartbeatAt  *time.Time  `json:"last_heartbeat_at,omitempty"`
	MaxConcurrency   int         `json:"max_concurrency"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

// Execution is the execution-plane entity for one CampaignRun. planning = just ingested; running = jobs created.
type Execution struct {
	ID                       uuid.UUID       `json:"id"`
	RunID                    uuid.UUID       `json:"run_id"`
	CampaignID               uuid.UUID       `json:"campaign_id"`
	TargetID                 uuid.UUID       `json:"target_id"`
	TargetMaterializationID  uuid.UUID       `json:"target_materialization_id"`
	ScanProfileSlug          string          `json:"scan_profile_slug"`
	ScanProfileConfig       json.RawMessage `json:"scan_profile_config,omitempty"`
	Status                   string          `json:"status"`
	TotalJobs                int             `json:"total_jobs"`
	CompletedJobs            int             `json:"completed_jobs"`
	FailedJobs               int             `json:"failed_jobs"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
	CompletedAt              *time.Time      `json:"completed_at,omitempty"`
	ErrorSummary             string          `json:"error_summary,omitempty"`
}

// ExecutionJob is one assignable unit of work. Lease/assignment embedded (assigned_worker_id, lease_expires_at, lease_id).
type ExecutionJob struct {
	ID                 uuid.UUID       `json:"id"`
	ExecutionID        uuid.UUID       `json:"execution_id"`
	Payload            json.RawMessage `json:"payload"` // e.g. {"prefixes":["1.2.3.0/24"],"engine":"portscan-basic"}
	Status             string          `json:"status"`
	AssignedWorkerID   *uuid.UUID      `json:"assigned_worker_id,omitempty"`
	LeaseExpiresAt     *time.Time      `json:"lease_expires_at,omitempty"`
	LeaseID            string          `json:"lease_id,omitempty"`
	Attempt            int             `json:"attempt"`
	MaxAttempts        int             `json:"max_attempts"`
	ResultSummary      json.RawMessage `json:"result_summary,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	StartedAt          *time.Time      `json:"started_at,omitempty"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
}
