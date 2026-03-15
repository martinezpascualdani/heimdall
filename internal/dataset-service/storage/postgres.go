package storage

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/dataset-service/domain"
	_ "github.com/lib/pq"
)

// PostgresStore implements dataset version and artifact metadata storage.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore opens a connection and ensures schema.
func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if err := migrateDatasetService(db); err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func migrateDatasetService(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS dataset_versions (
			id UUID PRIMARY KEY,
			registry VARCHAR(32) NOT NULL DEFAULT '',
			serial BIGINT NOT NULL DEFAULT 0,
			start_date VARCHAR(16),
			end_date VARCHAR(16),
			record_count BIGINT DEFAULT 0,
			checksum VARCHAR(64),
			state VARCHAR(32) NOT NULL,
			storage_path TEXT,
			error_text TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_dataset_versions_registry_serial_validated
			ON dataset_versions(registry, serial) WHERE state = 'validated';
		CREATE INDEX IF NOT EXISTS idx_dataset_versions_registry ON dataset_versions(registry);
		CREATE INDEX IF NOT EXISTS idx_dataset_versions_state ON dataset_versions(state);
	`)
	if err != nil {
		return err
	}
	// CAIDA: source_type, source, source_version; RIR backfill
	_, _ = db.Exec(`ALTER TABLE dataset_versions ADD COLUMN IF NOT EXISTS source_type VARCHAR(8) NOT NULL DEFAULT 'rir'`)
	_, _ = db.Exec(`ALTER TABLE dataset_versions ADD COLUMN IF NOT EXISTS source VARCHAR(64) NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE dataset_versions ADD COLUMN IF NOT EXISTS source_version VARCHAR(512) NOT NULL DEFAULT ''`)
	// Backfill existing RIR rows so source = registry
	_, _ = db.Exec(`UPDATE dataset_versions SET source = registry, source_type = 'rir' WHERE source = '' OR source IS NULL`)
	// Unique index for CAIDA: (source, source_version) WHERE state = 'validated' AND source_type = 'caida'
	_, err = db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_dataset_versions_caida_validated
			ON dataset_versions(source, source_version) WHERE state = 'validated' AND source_type = 'caida';
		CREATE INDEX IF NOT EXISTS idx_dataset_versions_source ON dataset_versions(source);
	`)
	return err
}

// CreateVersion inserts a new dataset version (e.g. state fetching).
func (s *PostgresStore) CreateVersion(v *domain.DatasetVersion) error {
	_, err := s.db.Exec(`
		INSERT INTO dataset_versions (id, registry, serial, source_type, source, source_version, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`,
		v.ID, v.Registry, v.Serial, v.SourceType, v.Source, v.SourceVersion, v.StartDate, v.EndDate, v.RecordCount, v.Checksum, v.State, v.StoragePath, v.Error, v.CreatedAt, v.UpdatedAt)
	return err
}

// GetByID returns a version by ID.
func (s *PostgresStore) GetByID(id uuid.UUID) (*domain.DatasetVersion, error) {
	var v domain.DatasetVersion
	err := s.db.QueryRow(`
		SELECT id, registry, serial, source_type, source, source_version, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions WHERE id = $1
	`, id).Scan(&v.ID, &v.Registry, &v.Serial, &v.SourceType, &v.Source, &v.SourceVersion, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetByRegistrySerial returns a version by natural key (registry, serial). RIR only.
// Prefers the validated row if multiple exist.
func (s *PostgresStore) GetByRegistrySerial(registry string, serial int64) (*domain.DatasetVersion, error) {
	var v domain.DatasetVersion
	err := s.db.QueryRow(`
		SELECT id, registry, serial, source_type, source, source_version, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions WHERE registry = $1 AND serial = $2
		ORDER BY CASE WHEN state = 'validated' THEN 0 ELSE 1 END
		LIMIT 1
	`, registry, serial).Scan(&v.ID, &v.Registry, &v.Serial, &v.SourceType, &v.Source, &v.SourceVersion, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// UpdateState updates state and optionally storage path and error.
func (s *PostgresStore) UpdateState(id uuid.UUID, state domain.DatasetVersionState, storagePath, errText string) error {
	_, err := s.db.Exec(`
		UPDATE dataset_versions SET state = $1, updated_at = $2, storage_path = COALESCE(NULLIF($3, ''), storage_path), error_text = $4 WHERE id = $5
	`, state, time.Now(), storagePath, errText, id)
	return err
}

// UpdateVersionMeta sets serial, dates, record count, and storage path (after validation).
func (s *PostgresStore) UpdateVersionMeta(id uuid.UUID, serial int64, startDate, endDate string, recordCount int64, storagePath string) error {
	_, err := s.db.Exec(`
		UPDATE dataset_versions SET serial = $1, start_date = $2, end_date = $3, record_count = $4, storage_path = $5, updated_at = $6 WHERE id = $7
	`, serial, startDate, endDate, recordCount, storagePath, time.Now(), id)
	return err
}

// List returns all versions, newest first. Optional filter by source or source_type.
func (s *PostgresStore) List(limit int, sourceFilter, sourceTypeFilter string) ([]*domain.DatasetVersion, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT id, registry, serial, source_type, source, source_version, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions WHERE 1=1`
	args := []interface{}{}
	n := 1
	if sourceFilter != "" {
		query += ` AND source = $` + strconv.Itoa(n)
		args = append(args, sourceFilter)
		n++
	}
	if sourceTypeFilter != "" {
		query += ` AND source_type = $` + strconv.Itoa(n)
		args = append(args, sourceTypeFilter)
		n++
	}
	query += ` ORDER BY created_at DESC LIMIT $` + strconv.Itoa(n)
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.DatasetVersion
	for rows.Next() {
		var v domain.DatasetVersion
		if err := rows.Scan(&v.ID, &v.Registry, &v.Serial, &v.SourceType, &v.Source, &v.SourceVersion, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// GetLatestByRegistry returns the latest validated version for a registry (RIR). Orders by serial DESC.
func (s *PostgresStore) GetLatestByRegistry(registry string) (*domain.DatasetVersion, error) {
	var v domain.DatasetVersion
	err := s.db.QueryRow(`
		SELECT id, registry, serial, source_type, source, source_version, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions WHERE registry = $1 AND state = $2 ORDER BY serial DESC LIMIT 1
	`, registry, domain.StateValidated).Scan(&v.ID, &v.Registry, &v.Serial, &v.SourceType, &v.Source, &v.SourceVersion, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetLatestBySource returns the latest validated version for a source (RIR or CAIDA).
// For RIR (source_type=rir): order by serial DESC. For CAIDA: order by created_at DESC (documented).
func (s *PostgresStore) GetLatestBySource(source string) (*domain.DatasetVersion, error) {
	var v domain.DatasetVersion
	// First try as RIR (source = registry): order by serial DESC
	err := s.db.QueryRow(`
		SELECT id, registry, serial, source_type, source, source_version, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions WHERE source = $1 AND state = $2 AND source_type = $3 ORDER BY serial DESC LIMIT 1
	`, source, domain.StateValidated, domain.SourceTypeRIR).Scan(&v.ID, &v.Registry, &v.Serial, &v.SourceType, &v.Source, &v.SourceVersion, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt)
	if err == nil {
		return &v, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	// CAIDA: order by created_at DESC (latest imported)
	err = s.db.QueryRow(`
		SELECT id, registry, serial, source_type, source, source_version, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions WHERE source = $1 AND state = $2 AND source_type = $3 ORDER BY created_at DESC LIMIT 1
	`, source, domain.StateValidated, domain.SourceTypeCAIDA).Scan(&v.ID, &v.Registry, &v.Serial, &v.SourceType, &v.Source, &v.SourceVersion, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetBySourceVersion returns a version by (source, source_version). For CAIDA idempotency.
func (s *PostgresStore) GetBySourceVersion(source, sourceVersion string) (*domain.DatasetVersion, error) {
	var v domain.DatasetVersion
	err := s.db.QueryRow(`
		SELECT id, registry, serial, source_type, source, source_version, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions WHERE source = $1 AND source_version = $2
		ORDER BY CASE WHEN state = 'validated' THEN 0 ELSE 1 END LIMIT 1
	`, source, sourceVersion).Scan(&v.ID, &v.Registry, &v.Serial, &v.SourceType, &v.Source, &v.SourceVersion, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// Ping checks database connectivity (for readiness).
func (s *PostgresStore) Ping() error {
	return s.db.Ping()
}

// Close closes the database connection.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}
