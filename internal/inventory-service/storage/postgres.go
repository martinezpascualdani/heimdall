package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/inventory-service/domain"
	_ "github.com/lib/pq"
)

// ErrAlreadyIngested is returned when (execution_id, job_id) already exists in ingestion_log.
var ErrAlreadyIngested = errors.New("job already ingested")

// Canonical keys (do not use alternatives for upserts):
// - Assets: UNIQUE(asset_type, identity_normalized). Use identity_normalized for lookups/upserts, not identity_value.
// - Exposures: UNIQUE(asset_id, exposure_type, key_protocol, key_port). Use key_protocol+key_port for unicidad, not exposure_key.

// PostgresStore implements inventory-service persistence.
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
	if err := migrateInventoryService(db); err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func migrateInventoryService(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS asset_types (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			code VARCHAR(64) NOT NULL UNIQUE,
			description TEXT DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS exposure_types (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			code VARCHAR(64) NOT NULL UNIQUE,
			description TEXT DEFAULT ''
		);
		INSERT INTO asset_types (code, description) VALUES ('host', 'Host identified by IP') ON CONFLICT (code) DO NOTHING;
		INSERT INTO exposure_types (code, description) VALUES ('tcp_port', 'TCP port open') ON CONFLICT (code) DO NOTHING;

		CREATE TABLE IF NOT EXISTS assets (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			asset_type VARCHAR(64) NOT NULL,
			identity_value TEXT NOT NULL,
			identity_normalized TEXT NOT NULL,
			identity_data JSONB,
			first_seen_at TIMESTAMPTZ NOT NULL,
			last_seen_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(asset_type, identity_normalized)
		);
		CREATE INDEX IF NOT EXISTS idx_assets_asset_type ON assets(asset_type);
		CREATE INDEX IF NOT EXISTS idx_assets_last_seen_at ON assets(last_seen_at);

		CREATE TABLE IF NOT EXISTS exposures (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
			exposure_type VARCHAR(64) NOT NULL,
			key_protocol TEXT,
			key_port INT,
			exposure_key TEXT NOT NULL,
			first_seen_at TIMESTAMPTZ NOT NULL,
			last_seen_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(asset_id, exposure_type, key_protocol, key_port)
		);
		CREATE INDEX IF NOT EXISTS idx_exposures_asset_id ON exposures(asset_id);
		CREATE INDEX IF NOT EXISTS idx_exposures_exposure_type ON exposures(exposure_type);

		CREATE TABLE IF NOT EXISTS observations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			execution_id UUID NOT NULL,
			job_id UUID NOT NULL,
			run_id UUID,
			campaign_id UUID,
			target_id UUID,
			target_materialization_id UUID,
			scan_profile_slug TEXT,
			asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
			exposure_id UUID NOT NULL REFERENCES exposures(id) ON DELETE CASCADE,
			observed_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			observation_metadata JSONB
		);
		CREATE INDEX IF NOT EXISTS idx_observations_execution_job ON observations(execution_id, job_id);
		CREATE INDEX IF NOT EXISTS idx_observations_asset_observed ON observations(asset_id, observed_at);
		CREATE INDEX IF NOT EXISTS idx_observations_run_id ON observations(run_id);
		CREATE INDEX IF NOT EXISTS idx_observations_campaign_id ON observations(campaign_id);
		CREATE INDEX IF NOT EXISTS idx_observations_target_id ON observations(target_id);
		CREATE INDEX IF NOT EXISTS idx_observations_observed_at ON observations(observed_at);
		ALTER TABLE observations ADD COLUMN IF NOT EXISTS observation_metadata JSONB;

		CREATE TABLE IF NOT EXISTS ingestion_log (
			execution_id UUID NOT NULL,
			job_id UUID NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (execution_id, job_id)
		);
	`)
	return err
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) Ping() error {
	return s.db.Ping()
}

func nullableUUID(u *uuid.UUID) interface{} {
	if u == nil || *u == uuid.Nil {
		return nil
	}
	return *u
}

// IngestionLogExists returns true if (execution_id, job_id) already exists in ingestion_log.
func (s *PostgresStore) IngestionLogExists(executionID, jobID uuid.UUID) (bool, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT 1 FROM ingestion_log WHERE execution_id = $1 AND job_id = $2
	`, executionID, jobID).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// InsertIngestionLog inserts (execution_id, job_id). Caller must ensure uniqueness (e.g. within transaction after checks).
func (s *PostgresStore) InsertIngestionLog(executionID, jobID uuid.UUID) error {
	_, err := s.db.Exec(`
		INSERT INTO ingestion_log (execution_id, job_id) VALUES ($1, $2)
	`, executionID, jobID)
	return err
}

// IngestObservation is a single observation for ingesta (ip, port, status from worker).
type IngestObservation struct {
	IP     string
	Port   int
	Status string
}

// IngestJobCompleted runs a full ingesta for one job in a transaction: checks idempotency, upserts assets/exposures, inserts observations, logs ingestion.
func (s *PostgresStore) IngestJobCompleted(executionID, jobID, runID, campaignID, targetID, targetMatID uuid.UUID, scanProfile string, observedAt time.Time, observations []IngestObservation) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRow(`SELECT 1 FROM ingestion_log WHERE execution_id = $1 AND job_id = $2`, executionID, jobID).Scan(&exists)
	if err == nil {
		return ErrAlreadyIngested
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	for _, obs := range observations {
		if obs.IP == "" {
			continue
		}
		identityVal := obs.IP
		identityNorm := identityVal
		var assetID uuid.UUID
		err = tx.QueryRow(`
			INSERT INTO assets (asset_type, identity_value, identity_normalized, first_seen_at, last_seen_at)
			VALUES ('host', $1, $2, $3, $3)
			ON CONFLICT (asset_type, identity_normalized) DO UPDATE SET last_seen_at = $3, updated_at = NOW()
			RETURNING id
		`, identityVal, identityNorm, observedAt).Scan(&assetID)
		if err != nil {
			return err
		}
		exposureKey := fmt.Sprintf("tcp/%d", obs.Port)
		var exposureID uuid.UUID
		err = tx.QueryRow(`
			INSERT INTO exposures (asset_id, exposure_type, key_protocol, key_port, exposure_key, first_seen_at, last_seen_at)
			VALUES ($1, 'tcp_port', 'tcp', $2, $3, $4, $4)
			ON CONFLICT (asset_id, exposure_type, key_protocol, key_port) DO UPDATE SET last_seen_at = $4, updated_at = NOW()
			RETURNING id
		`, assetID, obs.Port, exposureKey, observedAt).Scan(&exposureID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`
			INSERT INTO observations (execution_id, job_id, run_id, campaign_id, target_id, target_materialization_id, scan_profile_slug, asset_id, exposure_id, observed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, executionID, jobID, nullableUUID(&runID), nullableUUID(&campaignID), nullableUUID(&targetID), nullableUUID(&targetMatID), nullStr(scanProfile), assetID, exposureID, observedAt)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(`INSERT INTO ingestion_log (execution_id, job_id) VALUES ($1, $2)`, executionID, jobID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// GetAsset returns asset by id, or nil if not found.
func (s *PostgresStore) GetAsset(id uuid.UUID) (*domain.Asset, error) {
	var a domain.Asset
	var identityData []byte
	err := s.db.QueryRow(`
		SELECT id, asset_type, identity_value, identity_normalized, identity_data, first_seen_at, last_seen_at, created_at, updated_at
		FROM assets WHERE id = $1
	`, id).Scan(
		&a.ID, &a.AssetType, &a.IdentityValue, &a.IdentityNormalized, &identityData,
		&a.FirstSeenAt, &a.LastSeenAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(identityData) > 0 {
		a.IdentityData = identityData
	}
	return &a, nil
}

// GetAssetByTypeIdentity returns the asset for the given asset_type and identity_normalized, or nil if not found.
func (s *PostgresStore) GetAssetByTypeIdentity(assetType, identityNormalized string) (*domain.Asset, error) {
	var a domain.Asset
	var identityData []byte
	err := s.db.QueryRow(`
		SELECT id, asset_type, identity_value, identity_normalized, identity_data, first_seen_at, last_seen_at, created_at, updated_at
		FROM assets WHERE asset_type = $1 AND identity_normalized = $2
	`, assetType, identityNormalized).Scan(
		&a.ID, &a.AssetType, &a.IdentityValue, &a.IdentityNormalized, &identityData,
		&a.FirstSeenAt, &a.LastSeenAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(identityData) > 0 {
		a.IdentityData = identityData
	}
	return &a, nil
}

// UpsertAsset inserts or updates asset by (asset_type, identity_normalized). If exists, updates last_seen_at; observedAt used for first_seen/last_seen on insert.
func (s *PostgresStore) UpsertAsset(assetType, identityValue, identityNormalized string, identityData []byte, observedAt time.Time) (*domain.Asset, error) {
	now := time.Now()
	_, err := s.db.Exec(`
		INSERT INTO assets (asset_type, identity_value, identity_normalized, identity_data, first_seen_at, last_seen_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5, $6, $6)
		ON CONFLICT (asset_type, identity_normalized) DO UPDATE SET
			last_seen_at = $5,
			updated_at = $6
	`, assetType, identityValue, identityNormalized, jsonbOrNull(identityData), observedAt, now)
	if err != nil {
		return nil, err
	}
	return s.GetAssetByTypeIdentity(assetType, identityNormalized)
}

func jsonbOrNull(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

// GetExposureByAssetAndKey returns exposure by asset_id, exposure_type, key_protocol, key_port (nullable for port).
func (s *PostgresStore) GetExposureByAssetAndKey(assetID uuid.UUID, exposureType, keyProtocol string, keyPort *int) (*domain.Exposure, error) {
	var e domain.Exposure
	var keyPortVal sql.NullInt32
	var keyPortArg interface{}
	if keyPort != nil {
		keyPortArg = *keyPort
	}
	err := s.db.QueryRow(`
		SELECT id, asset_id, exposure_type, key_protocol, key_port, exposure_key, first_seen_at, last_seen_at, created_at, updated_at
		FROM exposures WHERE asset_id = $1 AND exposure_type = $2 AND key_protocol IS NOT DISTINCT FROM $3 AND key_port IS NOT DISTINCT FROM $4
	`, assetID, exposureType, nullStr(keyProtocol), keyPortArg).Scan(
		&e.ID, &e.AssetID, &e.ExposureType, &e.KeyProtocol, &keyPortVal, &e.ExposureKey,
		&e.FirstSeenAt, &e.LastSeenAt, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if keyPortVal.Valid {
		p := int(keyPortVal.Int32)
		e.KeyPort = &p
	}
	return &e, nil
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// UpsertExposure inserts or updates exposure by (asset_id, exposure_type, key_protocol, key_port). observedAt for first_seen/last_seen.
func (s *PostgresStore) UpsertExposure(assetID uuid.UUID, exposureType, keyProtocol, exposureKey string, keyPort *int, observedAt time.Time) (*domain.Exposure, error) {
	now := time.Now()
	var keyPortVal interface{}
	if keyPort != nil {
		keyPortVal = *keyPort
	}
	var id uuid.UUID
	err := s.db.QueryRow(`
		INSERT INTO exposures (asset_id, exposure_type, key_protocol, key_port, exposure_key, first_seen_at, last_seen_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $7)
		ON CONFLICT (asset_id, exposure_type, key_protocol, key_port) DO UPDATE SET
			last_seen_at = $6,
			updated_at = $7
		RETURNING id
	`, assetID, exposureType, nullStr(keyProtocol), keyPortVal, exposureKey, observedAt, now).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetExposureByID(id)
}

// GetExposureByID returns exposure by id.
func (s *PostgresStore) GetExposureByID(id uuid.UUID) (*domain.Exposure, error) {
	var e domain.Exposure
	var keyPortVal sql.NullInt32
	err := s.db.QueryRow(`
		SELECT id, asset_id, exposure_type, key_protocol, key_port, exposure_key, first_seen_at, last_seen_at, created_at, updated_at
		FROM exposures WHERE id = $1
	`, id).Scan(
		&e.ID, &e.AssetID, &e.ExposureType, &e.KeyProtocol, &keyPortVal, &e.ExposureKey,
		&e.FirstSeenAt, &e.LastSeenAt, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if keyPortVal.Valid {
		p := int(keyPortVal.Int32)
		e.KeyPort = &p
	}
	return &e, nil
}

// InsertObservation inserts one observation row.
func (s *PostgresStore) InsertObservation(o *domain.Observation) error {
	o.CreatedAt = time.Now()
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	_, err := s.db.Exec(`
		INSERT INTO observations (id, execution_id, job_id, run_id, campaign_id, target_id, target_materialization_id, scan_profile_slug, asset_id, exposure_id, observed_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, o.ID, o.ExecutionID, o.JobID, nullableUUID(&o.RunID), nullableUUID(&o.CampaignID), nullableUUID(&o.TargetID), nullableUUID(&o.TargetMaterializationID), nullStr(o.ScanProfileSlug), o.AssetID, o.ExposureID, o.ObservedAt, o.CreatedAt)
	return err
}

// ListAssets returns paginated assets with optional filters.
func (s *PostgresStore) ListAssets(assetType string, campaignID, targetID, runID *uuid.UUID, firstSeenAfter, lastSeenAfter *time.Time, limit, offset int) ([]*domain.Asset, int, error) {
	var args []interface{}
	argNum := 0
	where := " FROM assets a WHERE 1=1"
	if assetType != "" {
		argNum++
		args = append(args, assetType)
		where += fmt.Sprintf(" AND a.asset_type = $%d", argNum)
	}
	if campaignID != nil || targetID != nil || runID != nil {
		subq := " AND EXISTS (SELECT 1 FROM observations o WHERE o.asset_id = a.id"
		if campaignID != nil {
			argNum++
			args = append(args, *campaignID)
			subq += fmt.Sprintf(" AND o.campaign_id = $%d", argNum)
		}
		if targetID != nil {
			argNum++
			args = append(args, *targetID)
			subq += fmt.Sprintf(" AND o.target_id = $%d", argNum)
		}
		if runID != nil {
			argNum++
			args = append(args, *runID)
			subq += fmt.Sprintf(" AND o.run_id = $%d", argNum)
		}
		where += subq + ")"
	}
	if firstSeenAfter != nil {
		argNum++
		args = append(args, *firstSeenAfter)
		where += fmt.Sprintf(" AND a.first_seen_at >= $%d", argNum)
	}
	if lastSeenAfter != nil {
		argNum++
		args = append(args, *lastSeenAfter)
		where += fmt.Sprintf(" AND a.last_seen_at >= $%d", argNum)
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*)"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	argNum++
	args = append(args, limit)
	argNum++
	args = append(args, offset)
	q := "SELECT a.id, a.asset_type, a.identity_value, a.identity_normalized, a.identity_data, a.first_seen_at, a.last_seen_at, a.created_at, a.updated_at" + where + fmt.Sprintf(" ORDER BY a.last_seen_at DESC LIMIT $%d OFFSET $%d", argNum-1, argNum)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Asset
	for rows.Next() {
		var a domain.Asset
		var identityData []byte
		if err := rows.Scan(&a.ID, &a.AssetType, &a.IdentityValue, &a.IdentityNormalized, &identityData, &a.FirstSeenAt, &a.LastSeenAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if len(identityData) > 0 {
			a.IdentityData = identityData
		}
		out = append(out, &a)
	}
	return out, total, rows.Err()
}

// ListExposuresByAssetID returns exposures for an asset (no pagination for subresource).
func (s *PostgresStore) ListExposuresByAssetID(assetID uuid.UUID) ([]*domain.Exposure, error) {
	rows, err := s.db.Query(`
		SELECT id, asset_id, exposure_type, key_protocol, key_port, exposure_key, first_seen_at, last_seen_at, created_at, updated_at
		FROM exposures WHERE asset_id = $1 ORDER BY exposure_type, key_port NULLS LAST, exposure_key
	`, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExposures(rows)
}

// ListExposures returns paginated exposures with optional filters.
func (s *PostgresStore) ListExposures(assetID *uuid.UUID, assetType, exposureType string, campaignID, targetID *uuid.UUID, limit, offset int) ([]*domain.Exposure, int, error) {
	var args []interface{}
	argNum := 0
	where := " FROM exposures e WHERE 1=1"
	if assetID != nil {
		argNum++
		args = append(args, *assetID)
		where += fmt.Sprintf(" AND e.asset_id = $%d", argNum)
	}
	if assetType != "" {
		argNum++
		args = append(args, assetType)
		where += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM assets a WHERE a.id = e.asset_id AND a.asset_type = $%d)", argNum)
	}
	if exposureType != "" {
		argNum++
		args = append(args, exposureType)
		where += fmt.Sprintf(" AND e.exposure_type = $%d", argNum)
	}
	if campaignID != nil || targetID != nil {
		subq := " AND EXISTS (SELECT 1 FROM observations o WHERE o.exposure_id = e.id"
		if campaignID != nil {
			argNum++
			args = append(args, *campaignID)
			subq += fmt.Sprintf(" AND o.campaign_id = $%d", argNum)
		}
		if targetID != nil {
			argNum++
			args = append(args, *targetID)
			subq += fmt.Sprintf(" AND o.target_id = $%d", argNum)
		}
		where += subq + ")"
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*)"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	argNum++
	args = append(args, limit)
	argNum++
	args = append(args, offset)
	q := "SELECT e.id, e.asset_id, e.exposure_type, e.key_protocol, e.key_port, e.exposure_key, e.first_seen_at, e.last_seen_at, e.created_at, e.updated_at" + where + fmt.Sprintf(" ORDER BY e.last_seen_at DESC LIMIT $%d OFFSET $%d", argNum-1, argNum)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list, err := scanExposures(rows)
	return list, total, err
}

func scanExposures(rows *sql.Rows) ([]*domain.Exposure, error) {
	var out []*domain.Exposure
	for rows.Next() {
		var e domain.Exposure
		var keyPortVal sql.NullInt32
		if err := rows.Scan(&e.ID, &e.AssetID, &e.ExposureType, &e.KeyProtocol, &keyPortVal, &e.ExposureKey, &e.FirstSeenAt, &e.LastSeenAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		if keyPortVal.Valid {
			p := int(keyPortVal.Int32)
			e.KeyPort = &p
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// ListObservations returns paginated observations with optional filters.
func (s *PostgresStore) ListObservations(executionID, jobID, runID, campaignID, targetID, assetID *uuid.UUID, fromTime, toTime *time.Time, limit, offset int) ([]*domain.Observation, int, error) {
	var args []interface{}
	argNum := 0
	where := " FROM observations o WHERE 1=1"
	if executionID != nil {
		argNum++
		args = append(args, *executionID)
		where += fmt.Sprintf(" AND o.execution_id = $%d", argNum)
	}
	if jobID != nil {
		argNum++
		args = append(args, *jobID)
		where += fmt.Sprintf(" AND o.job_id = $%d", argNum)
	}
	if runID != nil {
		argNum++
		args = append(args, *runID)
		where += fmt.Sprintf(" AND o.run_id = $%d", argNum)
	}
	if campaignID != nil {
		argNum++
		args = append(args, *campaignID)
		where += fmt.Sprintf(" AND o.campaign_id = $%d", argNum)
	}
	if targetID != nil {
		argNum++
		args = append(args, *targetID)
		where += fmt.Sprintf(" AND o.target_id = $%d", argNum)
	}
	if assetID != nil {
		argNum++
		args = append(args, *assetID)
		where += fmt.Sprintf(" AND o.asset_id = $%d", argNum)
	}
	if fromTime != nil {
		argNum++
		args = append(args, *fromTime)
		where += fmt.Sprintf(" AND o.observed_at >= $%d", argNum)
	}
	if toTime != nil {
		argNum++
		args = append(args, *toTime)
		where += fmt.Sprintf(" AND o.observed_at <= $%d", argNum)
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*)"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	argNum++
	args = append(args, limit)
	argNum++
	args = append(args, offset)
	q := "SELECT o.id, o.execution_id, o.job_id, o.run_id, o.campaign_id, o.target_id, o.target_materialization_id, o.scan_profile_slug, o.asset_id, o.exposure_id, o.observed_at, o.created_at" + where + fmt.Sprintf(" ORDER BY o.observed_at DESC LIMIT $%d OFFSET $%d", argNum-1, argNum)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*domain.Observation
	for rows.Next() {
		var o domain.Observation
		var runID, campaignID, targetID, targetMatID []byte
		err := rows.Scan(&o.ID, &o.ExecutionID, &o.JobID, &runID, &campaignID, &targetID, &targetMatID, &o.ScanProfileSlug, &o.AssetID, &o.ExposureID, &o.ObservedAt, &o.CreatedAt)
		if err != nil {
			return nil, 0, err
		}
		if len(runID) == 16 {
			copy(o.RunID[:], runID)
		}
		if len(campaignID) == 16 {
			copy(o.CampaignID[:], campaignID)
		}
		if len(targetID) == 16 {
			copy(o.TargetID[:], targetID)
		}
		if len(targetMatID) == 16 {
			copy(o.TargetMaterializationID[:], targetMatID)
		}
		out = append(out, &o)
	}
	return out, total, rows.Err()
}

// ObservationPair is (asset_id, exposure_id) for diff computation.
type ObservationPair struct {
	AssetID    uuid.UUID
	ExposureID uuid.UUID
}

// ListObservationPairsByExecution returns distinct (asset_id, exposure_id) pairs observed in the given execution.
func (s *PostgresStore) ListObservationPairsByExecution(executionID uuid.UUID) ([]ObservationPair, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT asset_id, exposure_id FROM observations WHERE execution_id = $1
	`, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ObservationPair
	for rows.Next() {
		var p ObservationPair
		if err := rows.Scan(&p.AssetID, &p.ExposureID); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
