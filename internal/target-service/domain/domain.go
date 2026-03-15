package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Target is the main logical resource: a reusable definition that produces materialized snapshots.
type Target struct {
	ID                   uuid.UUID       `json:"id"`
	Name                 string          `json:"name"`
	Description          string          `json:"description,omitempty"`
	Active               bool            `json:"active"`
	MaterializationPolicy json.RawMessage `json:"materialization_policy,omitempty"`
	Tags                 []string        `json:"tags,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// TargetRule is a first-class auditable rule: "include country ES", "exclude asn 12345", etc.
// selector_value for asn is stored as string but must represent a valid numeric ASN.
type TargetRule struct {
	ID            uuid.UUID `json:"id"`
	TargetID      uuid.UUID `json:"target_id"`
	Kind          string    `json:"kind"`           // include | exclude
	SelectorType  string    `json:"selector_type"`  // country | asn | prefix | world
	SelectorValue string   `json:"selector_value"` // ISO country, ASN (string), CIDR, or empty for world
	AddressFamily string   `json:"address_family,omitempty"` // ipv4 | ipv6 | all
	RuleOrder     int      `json:"rule_order"`
	CreatedAt     time.Time `json:"created_at"`
}

const (
	MaterializationStatusRunning   = "running"
	MaterializationStatusCompleted = "completed"
	MaterializationStatusFailed    = "failed"
)

// SnapshotRef is the minimum structure for scope_snapshot_ref / routing_snapshot_ref (reproducibility).
type SnapshotRef struct {
	Service       string    `json:"service"`        // scope | routing
	Endpoint      string    `json:"endpoint"`      // e.g. "GET /v1/scopes/country/{cc}/blocks"
	DatasetIDs    []string  `json:"dataset_ids"`   // UUIDs of datasets used
	ResolvedAt    time.Time `json:"resolved_at"`   // when those inputs were resolved
}

// TargetMaterialization is an immutable snapshot: once materialized it is never edited.
// materialized_at is the canonical timestamp for this snapshot.
type TargetMaterialization struct {
	ID                 uuid.UUID       `json:"id"`
	TargetID           uuid.UUID       `json:"target_id"`
	MaterializedAt     time.Time       `json:"materialized_at"`
	TotalPrefixCount   int             `json:"total_prefix_count"`
	Status             string          `json:"status"` // running | completed | failed
	ErrorMessage       string          `json:"error_message,omitempty"`
	StatusDetail       string          `json:"status_detail,omitempty"`
	ScopeSnapshotRef   json.RawMessage `json:"scope_snapshot_ref"`
	RoutingSnapshotRef json.RawMessage `json:"routing_snapshot_ref"`
}

// TargetEntry is one CIDR prefix in a materialization (immutable snapshot content).
type TargetEntry struct {
	MaterializationID uuid.UUID `json:"materialization_id"`
	Prefix            string    `json:"prefix"` // CIDR
}
