package domain

import (
	"time"

	"github.com/google/uuid"
)

// Asset type codes (v1).
const (
	AssetTypeHost = "host"
)

// Exposure type codes (v1).
const (
	ExposureTypeTCPPort = "tcp_port"
)

// Asset is the canonical inventory entity (e.g. host by IP).
type Asset struct {
	ID                uuid.UUID `json:"id"`
	AssetType         string    `json:"asset_type"`
	IdentityValue     string    `json:"identity_value"`
	IdentityNormalized string    `json:"identity_normalized"`
	IdentityData      []byte    `json:"identity_data,omitempty"` // optional JSONB
	FirstSeenAt       time.Time `json:"first_seen_at"`
	LastSeenAt        time.Time `json:"last_seen_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Exposure is something observable on an asset (e.g. TCP port open).
type Exposure struct {
	ID           uuid.UUID `json:"id"`
	AssetID      uuid.UUID `json:"asset_id"`
	ExposureType string    `json:"exposure_type"`
	KeyProtocol  string    `json:"key_protocol,omitempty"` // e.g. "tcp"
	KeyPort      *int      `json:"key_port,omitempty"`
	ExposureKey  string    `json:"exposure_key"` // e.g. "tcp/443"
	FirstSeenAt  time.Time `json:"first_seen_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Observation is an immutable fact: one row = one (asset_id, exposure_id) observed in a given job at observed_at.
// We do not collapse multiple raw results into one; each observation row is one such fact.
// Optional observation_metadata (JSONB) can later hold status, engine, protocol, or other per-observation data without schema change.
type Observation struct {
	ID                      uuid.UUID  `json:"id"`
	ExecutionID             uuid.UUID  `json:"execution_id"`
	JobID                   uuid.UUID  `json:"job_id"`
	RunID                   uuid.UUID  `json:"run_id"`
	CampaignID              uuid.UUID  `json:"campaign_id"`
	TargetID                uuid.UUID  `json:"target_id"`
	TargetMaterializationID uuid.UUID  `json:"target_materialization_id"`
	ScanProfileSlug         string     `json:"scan_profile_slug,omitempty"`
	AssetID                 uuid.UUID  `json:"asset_id"`
	ExposureID              uuid.UUID  `json:"exposure_id"`
	ObservedAt              time.Time  `json:"observed_at"`
	CreatedAt               time.Time  `json:"created_at"`
}
