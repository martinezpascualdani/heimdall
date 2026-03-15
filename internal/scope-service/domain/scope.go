package domain

import (
	"time"

	"github.com/google/uuid"
)

// ScopeImportState is the state of an import.
type ScopeImportState string

const (
	ImportStateRunning          ScopeImportState = "running"
	ImportStateImported         ScopeImportState = "imported"
	ImportStateAlreadyImported  ScopeImportState = "already_imported"
	ImportStateReimportedForced ScopeImportState = "reimported_forced"
	ImportStateFailed           ScopeImportState = "failed"
)

// ScopeImport records an import attempt (config efectiva participates in idempotency).
type ScopeImport struct {
	ID              uuid.UUID        `json:"id"`
	DatasetID       uuid.UUID        `json:"dataset_id"`
	Registry        string           `json:"registry,omitempty"` // e.g. ripencc, arin — used for "latest per registry" resolution
	ConfigEffective string           `json:"config_effective"`   // e.g. status_filter hash or JSON
	State           ScopeImportState `json:"state"`
	BlocksPersisted int64            `json:"blocks_persisted"`
	DurationMs      int64            `json:"duration_ms"`
	Error           string           `json:"error,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
}

// ScopeBlock is a block (IPv4 or IPv6 range) for a scope (scope_type + scope_value).
type ScopeBlock struct {
	ID               uuid.UUID `json:"id"`
	DatasetID        uuid.UUID `json:"dataset_id"`
	ScopeType        string    `json:"scope_type"`  // e.g. country
	ScopeValue       string    `json:"scope_value"` // e.g. ES
	AddressFamily    string    `json:"address_family"` // ipv4 | ipv6
	BlockRawIdentity string   `json:"block_raw_identity"` // start|count|status|cc or prefix|prefix_length|status|cc
	Start            string    `json:"start,omitempty"`
	Value            string    `json:"value,omitempty"` // count or prefix_length
	NormalizedCIDRs  []string  `json:"normalized_cidrs,omitempty"`
	Status           string    `json:"status"`
	CC               string    `json:"cc"`
	Date             string    `json:"date"`
	CreatedAt        time.Time `json:"created_at"`
}
