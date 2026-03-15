package storage

import (
	"database/sql"
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
			registry VARCHAR(32) NOT NULL,
			serial BIGINT NOT NULL,
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
	return err
}

// CreateVersion inserts a new dataset version (e.g. state fetching).
func (s *PostgresStore) CreateVersion(v *domain.DatasetVersion) error {
	_, err := s.db.Exec(`
		INSERT INTO dataset_versions (id, registry, serial, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`,
		v.ID, v.Registry, v.Serial, v.StartDate, v.EndDate, v.RecordCount, v.Checksum, v.State, v.StoragePath, v.Error, v.CreatedAt, v.UpdatedAt)
	return err
}

// GetByID returns a version by ID.
func (s *PostgresStore) GetByID(id uuid.UUID) (*domain.DatasetVersion, error) {
	var v domain.DatasetVersion
	err := s.db.QueryRow(`
		SELECT id, registry, serial, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions WHERE id = $1
	`, id).Scan(&v.ID, &v.Registry, &v.Serial, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetByRegistrySerial returns a version by natural key (registry, serial).
// Prefers the validated row if multiple exist (e.g. one failed attempt + one validated) to avoid duplicate key on insert.
func (s *PostgresStore) GetByRegistrySerial(registry string, serial int64) (*domain.DatasetVersion, error) {
	var v domain.DatasetVersion
	err := s.db.QueryRow(`
		SELECT id, registry, serial, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions WHERE registry = $1 AND serial = $2
		ORDER BY CASE WHEN state = 'validated' THEN 0 ELSE 1 END
		LIMIT 1
	`, registry, serial).Scan(&v.ID, &v.Registry, &v.Serial, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt)
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

// List returns all versions, newest first.
func (s *PostgresStore) List(limit int) ([]*domain.DatasetVersion, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT id, registry, serial, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.DatasetVersion
	for rows.Next() {
		var v domain.DatasetVersion
		if err := rows.Scan(&v.ID, &v.Registry, &v.Serial, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// GetLatestByRegistry returns the latest validated version for a registry.
func (s *PostgresStore) GetLatestByRegistry(registry string) (*domain.DatasetVersion, error) {
	var v domain.DatasetVersion
	err := s.db.QueryRow(`
		SELECT id, registry, serial, start_date, end_date, record_count, checksum, state, storage_path, error_text, created_at, updated_at
		FROM dataset_versions WHERE registry = $1 AND state = $2 ORDER BY serial DESC LIMIT 1
	`, registry, domain.StateValidated).Scan(&v.ID, &v.Registry, &v.Serial, &v.StartDate, &v.EndDate, &v.RecordCount, &v.Checksum, &v.State, &v.StoragePath, &v.Error, &v.CreatedAt, &v.UpdatedAt)
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
