package domain

import (
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

// DatasetVersion is a version of a delegated stats dataset (option A: always register attempt).
type DatasetVersion struct {
	ID          uuid.UUID           `json:"id"`
	Registry    string              `json:"registry"`
	Serial      int64               `json:"serial"`
	StartDate   string              `json:"start_date"`
	EndDate     string              `json:"end_date"`
	RecordCount int64               `json:"record_count"`
	Checksum    string              `json:"checksum,omitempty"`
	State       DatasetVersionState `json:"state"`
	StoragePath string              `json:"-"` // path or key for artifact
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	Error       string              `json:"error,omitempty"` // last error if rejected/failed
}

// DatasetArtifact is the physical artifact (file) for a version.
// dataset-service is the owner; scope-service accesses via API.
type DatasetArtifact struct {
	VersionID   uuid.UUID
	StoragePath string
	SizeBytes   int64
}
