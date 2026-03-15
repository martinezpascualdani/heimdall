package domain

import (
	"time"

	"github.com/google/uuid"
)

// BGPPrefixOrigin is one prefix->ASN row from pfx2as (BGP observed origin).
type BGPPrefixOrigin struct {
	ID           uuid.UUID `json:"id"`
	DatasetID    uuid.UUID `json:"dataset_id"`
	Source       string    `json:"source"`
	IPVersion    string    `json:"ip_version"` // ipv4 | ipv6
	Prefix       string    `json:"prefix"`    // CIDR form e.g. 1.0.0.0/24
	PrefixLength int       `json:"prefix_length"`
	ASNRaw       string    `json:"asn_raw"`     // original value from file
	PrimaryASN   *int64    `json:"primary_asn"` // first AS as simplification; nil if multi unparseable
	ASNType      string    `json:"asn_type"`    // single | multi
	CreatedAt    time.Time `json:"created_at"`
}

// ASNMetadata is ASN -> org/name from CAIDA AS Organizations.
type ASNMetadata struct {
	ASN              int64     `json:"asn"`
	ASName           *string   `json:"as_name,omitempty"`
	OrgID            *string   `json:"org_id,omitempty"`
	OrgName          *string   `json:"org_name,omitempty"`
	Source           string    `json:"source"`
	SourceDatasetID  *uuid.UUID `json:"source_dataset_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RoutingImportState is the state of an import run.
type RoutingImportState string

const (
	RoutingImportRunning    RoutingImportState = "running"
	RoutingImportImported   RoutingImportState = "imported"
	RoutingImportAlready    RoutingImportState = "already_imported"
	RoutingImportReimported RoutingImportState = "reimported_forced"
	RoutingImportFailed     RoutingImportState = "failed"
)

// RoutingImport records one import of a routing/metadata dataset.
type RoutingImport struct {
	ID             uuid.UUID          `json:"id"`
	DatasetID      uuid.UUID          `json:"dataset_id"`
	Source         string             `json:"source"` // caida_pfx2as_ipv4, caida_pfx2as_ipv6, caida_as_org
	State          RoutingImportState `json:"state"`
	RowsPersisted  int64              `json:"rows_persisted"`
	DurationMs     int64              `json:"duration_ms"`
	ErrorText      string             `json:"error_text,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
}
