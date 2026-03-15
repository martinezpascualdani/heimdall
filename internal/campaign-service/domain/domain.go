package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ScheduleType is manual | once | interval.
const (
	ScheduleTypeManual   = "manual"
	ScheduleTypeOnce     = "once"
	ScheduleTypeInterval = "interval"
)

// MaterializationPolicy is use_latest | rematerialize.
const (
	MaterializationPolicyUseLatest    = "use_latest"
	MaterializationPolicyRematerialize = "rematerialize"
)

// ConcurrencyPolicy is allow | forbid_if_active.
const (
	ConcurrencyPolicyAllow           = "allow"
	ConcurrencyPolicyForbidIfActive   = "forbid_if_active"
)

// Run status: control plane (v1) vs data plane (future).
const (
	StatusPending        = "pending"
	StatusDispatching    = "dispatching"
	StatusDispatched     = "dispatched"
	StatusDispatchFailed = "dispatch_failed"
	StatusCanceled       = "canceled"
	// Data plane (campaign-service may set completed only for noop/0-prefix in v1):
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Campaign is the persistent definition of what to run and when.
type Campaign struct {
	ID                   uuid.UUID       `json:"id"`
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	Active               bool            `json:"active"`
	TargetID             uuid.UUID       `json:"target_id"`
	ScanProfileID        uuid.UUID       `json:"scan_profile_id"`
	ScheduleType         string          `json:"schedule_type"`
	ScheduleConfig       json.RawMessage `json:"schedule_config,omitempty"`
	MaterializationPolicy string         `json:"materialization_policy"`
	NextRunAt            *time.Time      `json:"next_run_at,omitempty"`
	RunOnceDone          bool            `json:"run_once_done"`
	ConcurrencyPolicy    string          `json:"concurrency_policy"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// ScanProfile is a reusable scan profile (name, slug, future config).
type ScanProfile struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CampaignRun is an immutable execution record; freezes target snapshot and profile.
type CampaignRun struct {
	ID                        uuid.UUID       `json:"id"`
	CampaignID                uuid.UUID       `json:"campaign_id"`
	TargetID                  uuid.UUID       `json:"target_id"`
	TargetMaterializationID   uuid.UUID       `json:"target_materialization_id"`
	ScanProfileID             uuid.UUID       `json:"scan_profile_id"`
	ScanProfileSlug           string          `json:"scan_profile_slug"`
	ScanProfileConfigSnapshot json.RawMessage `json:"scan_profile_config_snapshot,omitempty"`
	Status                    string          `json:"status"`
	CreatedAt                 time.Time       `json:"created_at"`
	StartedAt                 *time.Time      `json:"started_at,omitempty"`
	CompletedAt               *time.Time      `json:"completed_at,omitempty"`
	DispatchedAt              *time.Time      `json:"dispatched_at,omitempty"`
	DispatchRef               string          `json:"dispatch_ref,omitempty"`
	ErrorMessage              string          `json:"error_message,omitempty"`
	Stats                     json.RawMessage `json:"stats,omitempty"`
}
