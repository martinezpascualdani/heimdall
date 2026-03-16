package events

import (
	"time"

	"github.com/google/uuid"
)

// JobCompletedObservation is the canonical type for one observation in a job_completed event.
type JobCompletedObservation struct {
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Status string `json:"status"`
}

// JobCompletedEvent is the canonical event payload for job_completed.
// Used for both serialization (execution-service outbox) and deserialization (inventory-service consumer).
// payload_version is always string (e.g. "1"); do not use int.
const (
	JobCompletedEventType    = "job_completed"
	JobCompletedPayloadVersion = "1"
)

type JobCompletedEvent struct {
	EventType               string                   `json:"event_type"`                 // "job_completed"
	PayloadVersion          string                   `json:"payload_version"`             // "1"
	ExecutionID             string                   `json:"execution_id"`
	JobID                   string                   `json:"job_id"`
	RunID                   string                   `json:"run_id"`
	CampaignID              string                   `json:"campaign_id"`
	TargetID                string                   `json:"target_id"`
	TargetMaterializationID string                   `json:"target_materialization_id"`
	ScanProfileSlug         string                   `json:"scan_profile_slug"`
	ObservedAt              time.Time                `json:"observed_at"` // RFC3339 in JSON
	Observations            []JobCompletedObservation `json:"observations"`
}

// OutboxEventRow is used by the outbox-publisher (id + payload bytes). Shared by execution-service storage and outbox package.
type OutboxEventRow struct {
	ID      uuid.UUID
	Payload []byte
}
