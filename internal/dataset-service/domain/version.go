package domain

import (
	"strconv"
	"time"

	"github.com/google/uuid"
)

// DatasetVersionState represents the lifecycle state of a dataset version.
type DatasetVersionState string

const (
	StateFetching   DatasetVersionState = "fetching"
	StateFetched    DatasetVersionState = "fetched"
	StateValidated  DatasetVersionState = "validated"
	StateRejected   DatasetVersionState = "rejected"
	StateFailed     DatasetVersionState = "failed"
)

// SourceType distinguishes RIR from CAIDA (and future) sources.
const (
	SourceTypeRIR   = "rir"
	SourceTypeCAIDA = "caida"
)

// DatasetVersion is a version of a dataset (RIR delegated stats or CAIDA).
// For RIR: source_type=rir, source=registry (e.g. ripencc), serial set, source_version empty.
// For CAIDA: source_type=caida, source=e.g. caida_pfx2as_ipv4, source_version set, serial nil/zero.
type DatasetVersion struct {
	ID            uuid.UUID           `json:"id"`
	Registry      string              `json:"registry,omitempty"` // kept for RIR API compat; equals Source for RIR
	Serial        int64               `json:"serial,omitempty"`   // only for RIR; 0 for CAIDA
	SourceType    string              `json:"source_type"`       // rir | caida
	Source        string              `json:"source"`             // e.g. ripencc, caida_pfx2as_ipv4
	SourceVersion string              `json:"source_version,omitempty"`
	StartDate     string              `json:"start_date"`
	EndDate       string              `json:"end_date"`
	RecordCount   int64               `json:"record_count"`
	Checksum      string              `json:"checksum,omitempty"`
	State         DatasetVersionState `json:"state"`
	StoragePath   string              `json:"-"` // path or key for artifact
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
	Error         string              `json:"error,omitempty"`
}

// SnapshotKey returns a unique application-level key for this version (idempotency / dedup).
// RIR: source + serial; CAIDA: source + source_version. Application abstraction only.
func (v *DatasetVersion) SnapshotKey() string {
	if v.SourceType == SourceTypeCAIDA {
		return v.Source + "::" + v.SourceVersion
	}
	return v.Source + "::" + strconv.FormatInt(v.Serial, 10)
}

// DatasetArtifact is the physical artifact (file) for a version.
// dataset-service is the owner; scope-service accesses via API.
type DatasetArtifact struct {
	VersionID   uuid.UUID
	StoragePath string
	SizeBytes   int64
}
