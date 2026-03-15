package storage

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/routing-service/domain"
	_ "github.com/lib/pq"
)

// PostgresStore implements routing-service persistence.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore opens a connection and runs migrations.
func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if err := migrateRoutingService(db); err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func migrateRoutingService(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS routing_imports (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			dataset_id UUID NOT NULL,
			source VARCHAR(64) NOT NULL,
			state VARCHAR(32) NOT NULL,
			rows_persisted BIGINT DEFAULT 0,
			duration_ms BIGINT DEFAULT 0,
			error_text TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_routing_imports_dataset ON routing_imports(dataset_id);
		CREATE INDEX IF NOT EXISTS idx_routing_imports_source ON routing_imports(source);

		CREATE TABLE IF NOT EXISTS bgp_prefix_origins (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			dataset_id UUID NOT NULL,
			source VARCHAR(64) NOT NULL,
			ip_version VARCHAR(4) NOT NULL,
			prefix CIDR NOT NULL,
			prefix_length INT NOT NULL,
			asn_raw TEXT NOT NULL,
			primary_asn BIGINT,
			asn_type VARCHAR(16) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(dataset_id, prefix)
		);
		CREATE INDEX IF NOT EXISTS idx_bgp_prefix_origins_dataset_version ON bgp_prefix_origins(dataset_id, ip_version);
		CREATE INDEX IF NOT EXISTS idx_bgp_prefix_origins_lpm ON bgp_prefix_origins(dataset_id, ip_version) INCLUDE (prefix, prefix_length, asn_raw, primary_asn, asn_type);

		CREATE TABLE IF NOT EXISTS asn_metadata (
			asn BIGINT PRIMARY KEY,
			as_name TEXT,
			org_id TEXT,
			org_name TEXT,
			source VARCHAR(64) NOT NULL,
			source_dataset_id UUID,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_asn_metadata_source ON asn_metadata(source);
	`)
	return err
}

// CreateRoutingImport inserts a routing import record.
func (s *PostgresStore) CreateRoutingImport(imp *domain.RoutingImport) error {
	_, err := s.db.Exec(`
		INSERT INTO routing_imports (id, dataset_id, source, state, rows_persisted, duration_ms, error_text, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, imp.ID, imp.DatasetID, imp.Source, imp.State, imp.RowsPersisted, imp.DurationMs, imp.ErrorText, imp.CreatedAt)
	return err
}

// UpdateRoutingImportState updates state, rows_persisted, duration_ms, error_text.
func (s *PostgresStore) UpdateRoutingImportState(id uuid.UUID, state domain.RoutingImportState, rowsPersisted, durationMs int64, errText string) error {
	_, err := s.db.Exec(`
		UPDATE routing_imports SET state = $1, rows_persisted = $2, duration_ms = $3, error_text = $4 WHERE id = $5
	`, state, rowsPersisted, durationMs, errText, id)
	return err
}

// FindRoutingImportByDatasetAndSource returns an import for (dataset_id, source) if any (e.g. for idempotency).
func (s *PostgresStore) FindRoutingImportByDatasetAndSource(datasetID uuid.UUID, source string) (*domain.RoutingImport, error) {
	var imp domain.RoutingImport
	err := s.db.QueryRow(`
		SELECT id, dataset_id, source, state, rows_persisted, duration_ms, error_text, created_at
		FROM routing_imports WHERE dataset_id = $1 AND source = $2
		ORDER BY created_at DESC LIMIT 1
	`, datasetID, source).Scan(&imp.ID, &imp.DatasetID, &imp.Source, &imp.State, &imp.RowsPersisted, &imp.DurationMs, &imp.ErrorText, &imp.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &imp, nil
}

// GetLatestImportedDatasetIDBySource returns the dataset_id of the most recent successfully imported version for the given source.
func (s *PostgresStore) GetLatestImportedDatasetIDBySource(source string) (*uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(`
		SELECT dataset_id FROM routing_imports
		WHERE source = $1 AND state IN ('imported', 'already_imported', 'reimported_forced')
		ORDER BY created_at DESC LIMIT 1
	`, source).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// HasImportedRoutingDataset returns true if the given dataset_id has been imported as a routing (pfx2as) source.
func (s *PostgresStore) HasImportedRoutingDataset(datasetID uuid.UUID) (bool, error) {
	var dummy int
	err := s.db.QueryRow(`
		SELECT 1 FROM routing_imports
		WHERE dataset_id = $1 AND source IN ('caida_pfx2as_ipv4', 'caida_pfx2as_ipv6') AND state IN ('imported', 'already_imported', 'reimported_forced')
		LIMIT 1
	`, datasetID).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// UpsertBGPPrefixOrigin inserts or updates one prefix row. Uniqueness (dataset_id, prefix).
func (s *PostgresStore) UpsertBGPPrefixOrigin(o *domain.BGPPrefixOrigin) error {
	_, err := s.db.Exec(`
		INSERT INTO bgp_prefix_origins (id, dataset_id, source, ip_version, prefix, prefix_length, asn_raw, primary_asn, asn_type, created_at)
		VALUES ($1, $2, $3, $4, $5::cidr, $6, $7, $8, $9, $10)
		ON CONFLICT (dataset_id, prefix) DO UPDATE SET
			asn_raw = EXCLUDED.asn_raw,
			primary_asn = EXCLUDED.primary_asn,
			asn_type = EXCLUDED.asn_type
	`, o.ID, o.DatasetID, o.Source, o.IPVersion, o.Prefix, o.PrefixLength, o.ASNRaw, nullableInt64(o.PrimaryASN), o.ASNType, o.CreatedAt)
	return err
}

func nullableInt64(p *int64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

// LongestPrefixMatch finds the most specific prefix containing the given IP for the dataset and ip_version.
// Query: WHERE dataset_id = $1 AND ip_version = $2 AND prefix >>= $3::inet ORDER BY masklen(prefix) DESC LIMIT 1
func (s *PostgresStore) LongestPrefixMatch(datasetID uuid.UUID, ipVersion, ip string) (*domain.BGPPrefixOrigin, error) {
	var o domain.BGPPrefixOrigin
	var primaryASN sql.NullInt64
	err := s.db.QueryRow(`
		SELECT id, dataset_id, source, ip_version, prefix::text, prefix_length, asn_raw, primary_asn, asn_type, created_at
		FROM bgp_prefix_origins
		WHERE dataset_id = $1 AND ip_version = $2 AND prefix >>= $3::inet
		ORDER BY masklen(prefix) DESC
		LIMIT 1
	`, datasetID, ipVersion, ip).Scan(&o.ID, &o.DatasetID, &o.Source, &o.IPVersion, &o.Prefix, &o.PrefixLength, &o.ASNRaw, &primaryASN, &o.ASNType, &o.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if primaryASN.Valid {
		v := primaryASN.Int64
		o.PrimaryASN = &v
	}
	return &o, nil
}

// ListPrefixesByPrimaryASN returns prefixes where primary_asn = asn for the given dataset, ordered by prefix_length then prefix. Pagination: limit, offset.
func (s *PostgresStore) ListPrefixesByPrimaryASN(datasetID uuid.UUID, asn int64, limit, offset int) ([]*domain.BGPPrefixOrigin, int, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM bgp_prefix_origins WHERE dataset_id = $1 AND primary_asn = $2`, datasetID, asn).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`
		SELECT id, dataset_id, source, ip_version, prefix::text, prefix_length, asn_raw, primary_asn, asn_type, created_at
		FROM bgp_prefix_origins
		WHERE dataset_id = $1 AND primary_asn = $2
		ORDER BY prefix_length ASC, prefix ASC
		LIMIT $3 OFFSET $4
	`, datasetID, asn, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []*domain.BGPPrefixOrigin
	for rows.Next() {
		var o domain.BGPPrefixOrigin
		var primaryASN sql.NullInt64
		if err := rows.Scan(&o.ID, &o.DatasetID, &o.Source, &o.IPVersion, &o.Prefix, &o.PrefixLength, &o.ASNRaw, &primaryASN, &o.ASNType, &o.CreatedAt); err != nil {
			return nil, 0, err
		}
		if primaryASN.Valid {
			v := primaryASN.Int64
			o.PrimaryASN = &v
		}
		list = append(list, &o)
	}
	return list, total, rows.Err()
}

// UpsertASNMetadata inserts or updates ASN metadata (by asn).
func (s *PostgresStore) UpsertASNMetadata(m *domain.ASNMetadata) error {
	now := time.Now()
	_, err := s.db.Exec(`
		INSERT INTO asn_metadata (asn, as_name, org_id, org_name, source, source_dataset_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (asn) DO UPDATE SET
			as_name = EXCLUDED.as_name,
			org_id = EXCLUDED.org_id,
			org_name = EXCLUDED.org_name,
			source = EXCLUDED.source,
			source_dataset_id = EXCLUDED.source_dataset_id,
			updated_at = EXCLUDED.updated_at
	`, m.ASN, m.ASName, m.OrgID, m.OrgName, m.Source, m.SourceDatasetID, now, now)
	return err
}

// GetASNMetadata returns metadata for the given ASN, or nil if not found.
func (s *PostgresStore) GetASNMetadata(asn int64) (*domain.ASNMetadata, error) {
	var m domain.ASNMetadata
	var sourceDatasetID []byte
	err := s.db.QueryRow(`
		SELECT asn, as_name, org_id, org_name, source, source_dataset_id, created_at, updated_at
		FROM asn_metadata WHERE asn = $1
	`, asn).Scan(&m.ASN, &m.ASName, &m.OrgID, &m.OrgName, &m.Source, &sourceDatasetID, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(sourceDatasetID) == 16 {
		var b [16]byte
		copy(b[:], sourceDatasetID)
		u := uuid.UUID(b)
		m.SourceDatasetID = &u
	}
	return &m, nil
}

// Ping checks database connectivity.
func (s *PostgresStore) Ping() error {
	return s.db.Ping()
}

// Close closes the database connection.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}
